import { useEffect, useState, type ReactNode } from "react";
import { api } from "./api/client";
import type { Health } from "./types";
import { useTheme } from "./hooks/useTheme";
import Topbar from "./components/Topbar";
import MissionControl from "./pages/MissionControl";
import WorkItemList from "./pages/WorkItemList";
import WorkItemDetail from "./pages/WorkItemDetail";
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
    page = (
      <Placeholder
        title="Projects"
        note="Registered projects, repositories, branches, validation commands and active WorkItems."
      />
    );
  } else if (hash === "#/agents") {
    page = <Placeholder title="Agents" note="Available agents, active sessions and their WorkItems." />;
  } else if (hash === "#/evidence") {
    page = (
      <Placeholder
        title="Evidence"
        note="Commit SHA, git diff, git diff --check, tests/race/vet, commands and exit codes."
      />
    );
  } else if (hash === "#/operations") {
    page = (
      <Placeholder
        title="Operations"
        note="Orchestrator health, deployed commit, services and OpenCode worker status."
      />
    );
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
