"""FastAPI server for the caller (autonomous workload) agent.

Endpoint: POST /run
  Body:    {"query": "<natural language question>"}
  No Authorization header required — the agent self-authenticates to the
  AgentGateway using a workload identity token obtained from Keycloak.

Endpoint: GET /health
  Returns: {"status": "ok"}

The agent demonstrates agent-to-agent authentication:
  1. Fetches a workload identity token from Keycloak (client_credentials or
     SA token exchange depending on USE_TOKEN_EXCHANGE).
  2. Calls the stock-agent via AgentGateway with that token in the
     Authorization header.
  3. Returns the stock-agent's response.

The AgentGateway validates that the token has aud=agentgateway and was issued
by the configured Keycloak realm before forwarding to the stock-agent.
"""

import logging
import os

import httpx
import uvicorn
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from .auth import token_provider

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("caller_agent.server")

_STOCK_AGENT_URL = os.environ.get(
    "STOCK_AGENT_URL",
    "http://agentgateway.agentgateway-system.svc.cluster.local:8080/agent/run",
)

app = FastAPI(title="Caller Agent", version="1.0.0")


class RunRequest(BaseModel):
    query: str


@app.post("/run")
async def run(body: RunRequest):
    try:
        token = await token_provider.get_token()

        async with httpx.AsyncClient(timeout=120.0) as client:
            resp = await client.post(
                _STOCK_AGENT_URL,
                json={"query": body.query},
                headers={"Authorization": f"Bearer {token}"},
            )
            resp.raise_for_status()

        return resp.json()
    except httpx.HTTPStatusError as exc:
        body = exc.response.text
        logger.error(
            "Stock agent returned %d — downstream error: %s",
            exc.response.status_code,
            body[:1000],
        )
        return JSONResponse(
            status_code=exc.response.status_code,
            content={"detail": body, "type": "HTTPStatusError"},
        )
    except Exception as exc:
        logger.exception("Caller agent run failed")
        return JSONResponse(
            status_code=500,
            content={"detail": str(exc), "type": type(exc).__name__},
        )


@app.get("/health")
def health():
    return {"status": "ok"}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.environ.get("PORT", "8080")))
