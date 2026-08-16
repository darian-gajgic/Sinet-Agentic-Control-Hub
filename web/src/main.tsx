import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

// Fonts are BUNDLED, never fetched: Vite resolves these to hashed .woff2 files
// under dist/assets and the binary embeds them, so a tailnet host with no way
// out to the internet renders the same type as one with (P3/CONVENTIONS.md
// §47). Inter arrives as one variable weight axis; JetBrains Mono only in the
// four weights the design uses, latin subset only.
import '@fontsource-variable/inter/wght.css'
import '@fontsource/jetbrains-mono/latin-400.css'
import '@fontsource/jetbrains-mono/latin-500.css'
import '@fontsource/jetbrains-mono/latin-600.css'
import '@fontsource/jetbrains-mono/latin-700.css'

import './index.css'
import App from './App.tsx'
import { ErrorBoundary } from './boundary.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {/* The last net (F1): a fault that escapes every inner fence still never
        black-screens — this one offers the full reload, because app-level
        state is gone with the crash. */}
    <ErrorBoundary grain="app">
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
