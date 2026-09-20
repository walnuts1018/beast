import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { LibraryPage } from '../pages/library'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <LibraryPage />
  </StrictMode>,
)
