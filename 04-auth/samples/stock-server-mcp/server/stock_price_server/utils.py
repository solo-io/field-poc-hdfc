"""Utility functions for the stock price MCP server."""

import yfinance as yf

VALID_PERIODS = {"1d", "5d", "1mo", "3mo", "6mo", "1y", "2y", "5y", "10y", "ytd", "max"}


def fetch_ticker(symbol: str) -> yf.Ticker:
    return yf.Ticker(symbol.upper())


def validate_period(period: str) -> str:
    if period not in VALID_PERIODS:
        raise ValueError(f"Invalid period '{period}'. Must be one of: {', '.join(sorted(VALID_PERIODS))}")
    return period
