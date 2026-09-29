import { useEffect, useState, type ReactNode } from "react";
import { api } from "./api/client";
import type { Health } from "./types";
import { useTheme } from "./hooks/useTheme";
import Topbar from "./components/Topbar";
import MissionControl from "./pages/MissionControl";
import WorkItemList from "./pages/WorkItemList";
import WorkItemDetail from "./pages/WorkItemDetail";
import EvidenceCenter from "./pages/EvidenceCenter";
import ProjectsCenter from "./pages/ProjectsCenter";
import AgentsCenter from "./pages/AgentsCenter";
import OperationsCenter from "./pages/OperationsCenter";
import Placeholder from "./pages/Placeholder";

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
  const [theme, toggleTheme] = useTheme();
  const [health, setHealth] = useState<Health | null>(null);

  useEffect(() => {
    let alive = true;
    const load = () =>
      api
        .health()
        .then((h) => alive && setHealth(h))
        .catch(() => alive && setHealth(null));
    load();
    const t = setInterval(load, 10000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  const wiMatch = hash.match(/^#\/workitems\/(.+)$/);
  let page: ReactNode;
  if (hash === "" || hash === "#/") {
    page = <MissionControl />;
  } else if (hash === "#/workitems") {
    page = <WorkItemList />;
  } else if (wiMatch) {
    page = <WorkItemDetail id={decodeURIComponent(wiMatch[1])} />;
  } else if (hash === "#/projects") {
    page = <ProjectsCenter />;
  } else if (hash === "#/agents") {
    page = <AgentsCenter />;
  } else if (hash === "#/evidence" || hash.startsWith("#/evidence/")) {
    const evMatch = hash.match(/^#\/evidence\/(.+)$/);
    page = <EvidenceCenter id={evMatch ? decodeURIComponent(evMatch[1]) : undefined} />;
  } else if (hash === "#/operations") {
    page = <OperationsCenter />;
  } else {
    page = <Placeholder title="Not found" note="Unknown route." />;
  }

  const current = hash === "" ? "#/" : hash;
  return (
    <div className="app">
      <Topbar health={health} theme={theme} onToggleTheme={toggleTheme} current={current} />
      <main className="content">{page}</main>
    </div>
  );
}
