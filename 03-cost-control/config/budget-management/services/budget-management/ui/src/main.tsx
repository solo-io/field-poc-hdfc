import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { Global } from '@emotion/react';
import { Toaster } from 'react-hot-toast';
import { globalStyles } from './styles';
import { AuthProvider } from './contexts/AuthContext';
import App from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Global styles={globalStyles} />
    <Toaster
      position="top-right"
      toastOptions={{
        duration: 4000,
        style: {
          background: '#11101C',
          color: '#FAFAFA',
          border: '1px solid #27242E',
        },
        success: {
          iconTheme: {
            primary: '#22C55E',
            secondary: '#FAFAFA',
          },
        },
        error: {
          iconTheme: {
            primary: '#EF4444',
            secondary: '#FAFAFA',
          },
        },
      }}
    />
    <BrowserRouter>
      <AuthProvider>
        <App />
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>
);
