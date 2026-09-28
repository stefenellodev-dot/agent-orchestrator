import { useEffect, useState } from "react";
import WorkItemList from "./pages/WorkItemList";
import WorkItemDetail from "./pages/WorkItemDetail";

function useHashRoute(): string {
  const [hash, setHash] = useState(window.location.hash);
  useEffect(() => {
    const onHash = () => setHash(window.location.hash);
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return hash;
}

export default function App() {
  const hash = useHashRoute();
  const match = hash.match(/^#\/workitems\/(.+)$/);

  return (
    <div className="app">
      <header className="topbar">
        <a href="#/" className="brand">
          Agent Orchestrator
        </a>
      </header>
      <main>{match ? <WorkItemDetail id={decodeURIComponent(match[1])} /> : <WorkItemList />}</main>
    </div>
  );
}
