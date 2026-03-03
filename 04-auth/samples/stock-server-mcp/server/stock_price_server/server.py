"""Main MCP server using FastMCP to expose stock price utilities."""

import logging
import os
from datetime import datetime

from starlette.requests import Request
from starlette.responses import JSONResponse
from fastmcp import FastMCP

from . import tools

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app: FastMCP = FastMCP("Stock Price Server")


@app.tool()
def get_stock_price(symbol: str) -> float:
    """Retrieve the current stock price for the given ticker symbol.
    Returns the latest closing price as a float, or -1.0 on failure."""
    return tools.get_stock_price(symbol)


@app.resource("stock://{symbol}")
def stock_resource(symbol: str) -> str:
    """Expose stock price data as a resource."""
    price = tools.get_stock_price(symbol)
    if price < 0:
        return f"Error: Could not retrieve price for symbol '{symbol}'."
    return f"The current price of '{symbol}' is ${price:.2f}."


@app.tool()
def get_stock_history(symbol: str, period: str = "1mo") -> str:
    """Retrieve historical data for a stock as a CSV formatted string.

    Parameters:
        symbol: The stock ticker symbol.
        period: The period over which to retrieve data (e.g., '1mo', '3mo', '1y').
    """
    return tools.get_stock_history(symbol, period)


@app.tool()
def compare_stocks(symbol1: str, symbol2: str) -> str:
    """Compare the current stock prices of two ticker symbols.
    Returns a formatted message comparing the two stock prices."""
    return tools.compare_stocks(symbol1, symbol2)


@app.custom_route("/health", methods=["GET"])
async def health_check(request: Request) -> JSONResponse:
    return JSONResponse({
        "status": "healthy",
        "server": "Stock Price Server",
        "timestamp": datetime.now().isoformat(),
        "tools": ["get_stock_price", "get_stock_history", "compare_stocks"],
    })


def main() -> None:
    transport = os.environ.get("MCP_TRANSPORT", "streamable-http")
    port = int(os.environ.get("MCP_PORT", "8000"))
    logger.info("Starting Stock Price MCP Server on http://0.0.0.0:%d (transport=%s)", port, transport)
    app.run(transport=transport, host="0.0.0.0", port=port)


if __name__ == "__main__":
    main()
