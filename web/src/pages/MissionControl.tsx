import { useEffect, useState } from "react";
import { loadMissionData, type MissionData } from "../lib/aggregate";
import SummaryBar from "../components/SummaryBar";
import HumanGatesPanel from "../components/HumanGatesPanel";
import Kanban from "../components/Kanban";
import SessionsPanel from "../components/SessionsPanel";
import ValidationsPanel from "../components/ValidationsPanel";
import ActivityFeed from "../components/ActivityFeed";

const REFRESH_MS = 5000;

export default function MissionControl() {
  const [data, setData] = useState<MissionData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    const tick = async () => {
      try {
        const d = await loadMissionData();
        if (alive) {
          setData(d);
          setError(null);
        }
      } catch (e) {
        if (alive) setError(String(e));
      }
    };
    tick();
    const t = setInterval(tick, REFRESH_MS);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  if (error && !data) return <p className="error">{error}</p>;
  if (!data) return <p className="muted">Loading Mission Control…</p>;

  return (
    <div className="mission">
      <SummaryBar data={data} />
      <HumanGatesPanel items={data.pendingGates} />
      <Kanban items={data.workitems} />
      <div className="grid-2">
        <SessionsPanel sessions={data.sessions} />
        <ValidationsPanel sessions={data.sessions} />
      </div>
      <ActivityFeed events={data.events} />
      {data.errors.length > 0 && <p className="muted small">Partial data: {data.errors.join("; ")}</p>}
    </div>
  );
}
