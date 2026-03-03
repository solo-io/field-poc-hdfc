"""Stock ADK agent definition.

Wires together:
  - MCPToolset: connects to the MCP server via StreamableHTTP.
  - LiteLlm: routes LLM calls through the agentgateway provider endpoint so
    the gateway handles provider auth, rate limiting, and telemetry.

MCP authentication is controlled by MCP_AUTH_MODE:
  propagate (default)
    ADKTokenPropagationPlugin forwards the incoming Authorization header to
    every outbound MCP call. Used when the JWT policy sits on the /mcp route
    and the caller's token should be validated there end-to-end.
  workload
    WorkloadMCPTokenProvider fetches this agent's own Keycloak token and
    injects it into every MCP call. Used in the workload-identity-chain use
    case where the stock-agent authenticates to MCP as its own identity.

Environment variables:
  MODEL          LLM model name forwarded in the request body (default: gemini-2.0-flash)
  LLM_BASE_URL   Agentgateway provider base URL (default: .../openai)
  MCP_URL        URL of the MCP server through the AGW proxy
  MCP_AUTH_MODE  'propagate' or 'workload' (default: propagate)
"""

import os

from google.adk.agents import LlmAgent
from google.adk.models.lite_llm import LiteLlm
from google.adk.tools.mcp_tool.mcp_toolset import MCPToolset, StreamableHTTPConnectionParams

_MCP_URL = os.environ.get(
    "MCP_URL",
    "http://agentgateway.agentgateway-system.svc.cluster.local:8080/mcp",
)
_LLM_BASE_URL = os.environ.get(
    "LLM_BASE_URL",
    "http://agentgateway.agentgateway-system.svc.cluster.local:8080/openai",
)
_MODEL = os.environ.get("MODEL", "gemini-2.0-flash")
_MCP_AUTH_MODE = os.environ.get("MCP_AUTH_MODE", "propagate")

plugin = None

if _MCP_AUTH_MODE == "workload":
    from .workload_auth import WorkloadMCPTokenProvider as _WorkloadMCPTokenProvider
    _mcp_header_provider = _WorkloadMCPTokenProvider().header_provider
else:
    from agentsts.adk import ADKTokenPropagationPlugin as _ADKTokenPropagationPlugin
    plugin = _ADKTokenPropagationPlugin()
    _mcp_header_provider = plugin.header_provider

toolset = MCPToolset(
    connection_params=StreamableHTTPConnectionParams(url=_MCP_URL),
    header_provider=_mcp_header_provider,
)

root_agent = LlmAgent(
    name="stock_agent",
    model=LiteLlm(
        model=f"openai/{_MODEL}",
        api_base=_LLM_BASE_URL,
        api_key="none",
    ),
    tools=[toolset],
    instruction=(
        "You are a financial assistant with access to real-time stock market tools. "
        "Use the get_stock_price tool when asked about stock prices. "
        "Always provide the stock symbol and the retrieved price in your response."
    ),
)
