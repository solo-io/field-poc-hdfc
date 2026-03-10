import { Routes, Route, Navigate } from 'react-router-dom';
import { AppLayout } from './components/layout/AppLayout';
import { ModelCostsPage } from './pages/ModelCosts/ModelCostsPage';
import { BudgetsPage } from './pages/Budgets/BudgetsPage';
import { BudgetDetailPage } from './pages/Budgets/BudgetDetailPage';

function App() {
  return (
    <AppLayout>
      <Routes>
        <Route path="/" element={<Navigate to="/model-costs" replace />} />
        <Route path="/model-costs" element={<ModelCostsPage />} />
        <Route path="/budgets" element={<BudgetsPage />} />
        <Route path="/budgets/:id" element={<BudgetDetailPage />} />
      </Routes>
    </AppLayout>
  );
}

export default App;
