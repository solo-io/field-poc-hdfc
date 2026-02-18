import os
import re
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
import webhook_api as api

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Opik client (lazy-initialised; tracing disabled when OPIK_API_KEY is unset)
# ---------------------------------------------------------------------------
_opik_client = None


def _init_opik():
    global _opik_client
    if not os.getenv("OPIK_API_KEY"):
        logger.info("OPIK_API_KEY not set — Opik tracing disabled")
        return
    try:
        import opik  # noqa: delayed import keeps startup fast when Opik is absent
        _opik_client = opik.Opik(
            project_name=os.getenv("OPIK_PROJECT_NAME", "agentgateway-guardrails"),
        )
        logger.info("Opik tracing enabled")
    except Exception as exc:
        logger.warning(f"Could not initialise Opik: {exc}")


# ---------------------------------------------------------------------------
# Opik evaluation metrics (Sentiment + Tone — heuristic, no LLM call)
# ---------------------------------------------------------------------------
_sentiment_metric = None
_tone_metric = None

TOXIC_PHRASES = [
    "you are stupid",
    "i hate you",
    "you are worthless",
    "kill yourself",
    "go die",
    "you are an idiot",
]

SENTIMENT_THRESHOLD = float(os.getenv("SENTIMENT_THRESHOLD", "-0.5"))


def _init_metrics():
    global _sentiment_metric, _tone_metric
    try:
        import nltk
        nltk.download("vader_lexicon", quiet=True)
    except Exception:
        pass
    try:
        from opik.evaluation.metrics import Sentiment, Tone
        _sentiment_metric = Sentiment()
        _tone_metric = Tone(forbidden_phrases=TOXIC_PHRASES)
        logger.info("Opik evaluation metrics initialised (Sentiment, Tone)")
    except Exception as exc:
        logger.warning(f"Opik metrics unavailable — falling back to pattern matching only: {exc}")


# ---------------------------------------------------------------------------
# PII regex patterns
# ---------------------------------------------------------------------------
PII_PATTERNS = {
    "credit_card": re.compile(r"\b(?:\d[ -]*?){13,16}\b"),
    "ssn": re.compile(r"\b\d{3}-\d{2}-\d{4}\b"),
    "email": re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b"),
    "phone": re.compile(r"\b(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b"),
}
MASK = "****"

BANNED_WORDS = [w.strip() for w in os.getenv("BANNED_WORDS", "violence,drugs,weapons,terrorism,exploit,abuse").split(",") if w.strip()]


# ---------------------------------------------------------------------------
# FastAPI lifespan
# ---------------------------------------------------------------------------
@asynccontextmanager
async def lifespan(_app: FastAPI):
    _init_opik()
    _init_metrics()
    yield
    if _opik_client:
        _opik_client.flush()


app = FastAPI(
    title="Opik Guardrail Webhook",
    version="0.1.0",
    description=(
        "Guardrail webhook server powered by Opik for AgentGateway. "
        "Implements the Solo.io Guardrail Webhook API contract with "
        "PII detection, toxicity analysis, banned-word filtering, and "
        "Opik-based Sentiment / Tone evaluation — all traced to Opik."
    ),
    lifespan=lifespan,
)


# ---------------------------------------------------------------------------
# Guardrail helpers
# ---------------------------------------------------------------------------
def _check_pii(text: str) -> dict[str, bool]:
    return {name: True for name, pat in PII_PATTERNS.items() if pat.search(text)}


def _mask_pii(text: str) -> str:
    for pat in PII_PATTERNS.values():
        text = pat.sub(MASK, text)
    return text


def _check_banned(text: str) -> str | None:
    lower = text.lower()
    return next((w for w in BANNED_WORDS if w in lower), None)


def _check_toxic(text: str) -> str | None:
    lower = text.lower()
    return next((p for p in TOXIC_PHRASES if p in lower), None)


def _check_sentiment(text: str) -> tuple[float, str | None]:
    if _sentiment_metric is None:
        return 0.0, None
    try:
        result = _sentiment_metric.score(output=text)
        return result.value, result.reason
    except Exception:
        return 0.0, None


def _check_tone(text: str) -> tuple[float, str | None]:
    if _tone_metric is None:
        return 1.0, None
    try:
        result = _tone_metric.score(output=text)
        return result.value, result.reason
    except Exception:
        return 1.0, None


# ---------------------------------------------------------------------------
# Opik trace helper
# ---------------------------------------------------------------------------
def _trace(name: str, input_data: dict, output_data: dict, spans: list[dict] | None = None):
    if not _opik_client:
        return
    try:
        trace = _opik_client.trace(name=name, input=input_data)
        for s in (spans or []):
            trace.span(
                name=s["name"],
                type="guardrail",
                input=s.get("input"),
                output=s.get("output"),
                metadata=s.get("metadata"),
            )
        trace.output = output_data
    except Exception as exc:
        logger.debug(f"Trace logging failed: {exc}")


# ---------------------------------------------------------------------------
# POST /request — pre-hook guardrail
# ---------------------------------------------------------------------------
@app.post("/request", response_model=api.GuardrailsPromptResponse, tags=["Webhooks"])
async def process_prompts(
    request: Request,
    req: api.GuardrailsPromptRequest,
) -> api.GuardrailsPromptResponse:
    logger.info("📥 Incoming /request webhook")
    should_mask = False
    spans: list[dict] = []

    for i, message in enumerate(req.body.messages):
        if not message.content:
            continue
        logger.info(f"→ Message[{i}] role={message.role}: {message.content}")
        content = message.content

        # --- toxic phrases ---
        toxic = _check_toxic(content)
        spans.append({"name": "toxic-phrases", "input": {"content": content}, "output": {"matched": toxic}})
        if toxic:
            logger.warning(f"⛔ RejectAction triggered: toxic phrase matched: '{toxic}'")
            out = {"action": "reject", "reason": f"toxic phrase: {toxic}"}
            _trace("guardrail-request", _prompt_input(req), out, spans)
            return api.GuardrailsPromptResponse(
                action=api.RejectAction(body=f"Rejected due to toxic language: matched phrase '{toxic}'", status_code=403, reason=f"toxic phrase: {toxic}"),
            )

        # --- banned words ---
        banned = _check_banned(content)
        spans.append({"name": "banned-words", "input": {"content": content}, "output": {"matched": banned}})
        if banned:
            logger.warning(f"⛔ RejectAction triggered: banned word matched: '{banned}'")
            out = {"action": "reject", "reason": f"banned word: {banned}"}
            _trace("guardrail-request", _prompt_input(req), out, spans)
            return api.GuardrailsPromptResponse(
                action=api.RejectAction(body=f"Rejected due to inappropriate content: matched word '{banned}'", status_code=403, reason=f"banned word: {banned}"),
            )

        # --- Opik sentiment ---
        compound, s_reason = _check_sentiment(content)
        spans.append({"name": "opik-sentiment", "input": {"content": content}, "output": {"score": compound, "reason": s_reason}, "metadata": {"threshold": SENTIMENT_THRESHOLD}})
        if compound < SENTIMENT_THRESHOLD:
            logger.warning(f"⛔ RejectAction triggered: negative sentiment ({compound:.3f})")
            out = {"action": "reject", "reason": f"sentiment: {compound:.3f}"}
            _trace("guardrail-request", _prompt_input(req), out, spans)
            return api.GuardrailsPromptResponse(
                action=api.RejectAction(body="Rejected due to negative sentiment in content", status_code=403, reason=f"sentiment score: {compound:.3f}"),
            )

        # --- Opik tone ---
        tone_score, t_reason = _check_tone(content)
        spans.append({"name": "opik-tone", "input": {"content": content}, "output": {"score": tone_score, "reason": t_reason}})
        if tone_score < 0.5:
            logger.warning(f"⛔ RejectAction triggered: problematic tone ({tone_score:.3f}): {t_reason}")
            out = {"action": "reject", "reason": f"tone: {t_reason}"}
            _trace("guardrail-request", _prompt_input(req), out, spans)
            return api.GuardrailsPromptResponse(
                action=api.RejectAction(body=f"Rejected due to tone issues: {t_reason}", status_code=403, reason=f"tone score: {tone_score:.3f}"),
            )

        # --- PII detection & masking ---
        pii = _check_pii(content)
        spans.append({"name": "pii-detection", "input": {"content": content}, "output": {"matches": list(pii.keys())}})
        if pii:
            for pii_type in pii:
                logger.info(f"🔒 Matched PII pattern: {pii_type}")
            masked = _mask_pii(content)
            logger.info(f"🔒 Masking content: {content} → {masked}")
            req.body.messages[i].content = masked
            should_mask = True

    if should_mask:
        logger.info("✅ MaskAction returned (request)")
        out = {"action": "mask", "reason": "PII detected and masked"}
        _trace("guardrail-request", _prompt_input(req), out, spans)
        return api.GuardrailsPromptResponse(action=api.MaskAction(body=req.body, reason="PII detected and masked"))

    logger.info("✅ PassAction returned (request)")
    out = {"action": "pass", "reason": "All checks passed"}
    _trace("guardrail-request", _prompt_input(req), out, spans)
    return api.GuardrailsPromptResponse(action=api.PassAction(reason="All checks passed"))


# ---------------------------------------------------------------------------
# POST /response — post-hook guardrail
# ---------------------------------------------------------------------------
@app.post("/response", response_model=api.GuardrailsResponseResponse, tags=["Webhooks"])
async def process_responses(
    request: Request,
    req: api.GuardrailsResponseRequest,
) -> api.GuardrailsResponseResponse:
    logger.info("📥 Incoming /response webhook")
    should_mask = False
    spans: list[dict] = []

    for i, choice in enumerate(req.body.choices):
        if not choice.message.content:
            continue
        logger.info(f"→ Choice[{i}] role={choice.message.role}: {choice.message.content}")
        content = choice.message.content

        # --- PII masking ---
        pii = _check_pii(content)
        spans.append({"name": "pii-detection", "input": {"content": content}, "output": {"matches": list(pii.keys())}})
        if pii:
            for pii_type in pii:
                logger.info(f"🔒 Matched PII in response: {pii_type}")
            masked = _mask_pii(content)
            logger.info(f"🔒 Masking response: {content} → {masked}")
            req.body.choices[i].message.content = masked
            should_mask = True

        # --- Opik sentiment ---
        compound, s_reason = _check_sentiment(content)
        spans.append({"name": "opik-sentiment", "input": {"content": content}, "output": {"score": compound, "reason": s_reason}})
        if compound < SENTIMENT_THRESHOLD:
            logger.warning(f"🔒 Masking response: negative sentiment ({compound:.3f})")
            req.body.choices[i].message.content = "[Content removed: safety policy violation]"
            should_mask = True

    if should_mask:
        logger.info("✅ MaskAction returned (response)")
        out = {"action": "mask", "reason": "Content masked in response"}
        _trace("guardrail-response", _response_input(req), out, spans)
        return api.GuardrailsResponseResponse(action=api.MaskAction(body=req.body, reason="Sensitive content masked"))

    logger.info("✅ PassAction returned (response)")
    out = {"action": "pass", "reason": "All checks passed"}
    _trace("guardrail-response", _response_input(req), out, spans)
    return api.GuardrailsResponseResponse(action=api.PassAction(reason="All checks passed"))


# ---------------------------------------------------------------------------
# Serialisation helpers
# ---------------------------------------------------------------------------
def _prompt_input(req: api.GuardrailsPromptRequest) -> dict:
    return {"messages": [m.model_dump() for m in req.body.messages]}


def _response_input(req: api.GuardrailsResponseRequest) -> dict:
    return {"choices": [c.message.model_dump() for c in req.body.choices]}
