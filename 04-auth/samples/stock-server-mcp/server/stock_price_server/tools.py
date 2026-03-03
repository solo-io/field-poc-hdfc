"""Stock price tools for the MCP server."""

from .utils import fetch_ticker, validate_period


def get_stock_price(symbol: str) -> float:
    """Retrieve the current stock price for the given ticker symbol.
    Returns the latest closing price as a float, or -1.0 on failure."""
    try:
        ticker = fetch_ticker(symbol)
        data = ticker.history(period="1d")
        if not data.empty:
            return float(data["Close"].iloc[-1])
        info = ticker.info
        price = info.get("regularMarketPrice")
        if price is not None:
            return float(price)
        return -1.0
    except Exception:
        return -1.0


def get_stock_history(symbol: str, period: str = "1mo") -> str:
    """Retrieve historical data for a stock as a CSV formatted string."""
    try:
        period = validate_period(period)
        ticker = fetch_ticker(symbol)
        data = ticker.history(period=period)
        if data.empty:
            return f"No historical data found for symbol '{symbol}' with period '{period}'."
        return data.to_csv()
    except Exception as e:
        return f"Error fetching historical data: {e}"


def compare_stocks(symbol1: str, symbol2: str) -> str:
    """Compare the current stock prices of two ticker symbols."""
    price1 = get_stock_price(symbol1)
    price2 = get_stock_price(symbol2)
    if price1 < 0 or price2 < 0:
        return f"Error: Could not retrieve data for comparison of '{symbol1}' and '{symbol2}'."
    if price1 > price2:
        return f"{symbol1} (${price1:.2f}) is higher than {symbol2} (${price2:.2f})."
    if price1 < price2:
        return f"{symbol1} (${price1:.2f}) is lower than {symbol2} (${price2:.2f})."
    return f"Both {symbol1} and {symbol2} have the same price (${price1:.2f})."
