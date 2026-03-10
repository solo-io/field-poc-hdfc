import { css } from '@emotion/react';
import { colors } from './colors';
import { fontFamily } from './typography';

export const globalStyles = css`
  *,
  *::before,
  *::after {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  html {
    font-size: 16px;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }

  body {
    font-family: ${fontFamily};
    background-color: ${colors.background};
    color: ${colors.foreground};
    line-height: 1.5;
    min-height: 100vh;
  }

  #root {
    min-height: 100vh;
    display: flex;
  }

  a {
    color: inherit;
    text-decoration: none;
  }

  button {
    font-family: inherit;
    cursor: pointer;
    border: none;
    background: none;
  }

  input,
  textarea,
  select {
    font-family: inherit;
  }

  table {
    border-collapse: collapse;
  }

  ::-webkit-scrollbar {
    width: 8px;
    height: 8px;
  }

  ::-webkit-scrollbar-track {
    background: ${colors.background};
  }

  ::-webkit-scrollbar-thumb {
    background: ${colors.border};
    border-radius: 4px;
  }

  ::-webkit-scrollbar-thumb:hover {
    background: ${colors.borderLight};
  }
`;
