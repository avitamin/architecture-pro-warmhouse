import test from 'node:test';
import assert from 'node:assert/strict';
import {observe, delay, http} from './observer.mjs';

test('GET reads saved history while disabled scheduler emits no PollRequested', {timeout: 20000}, async () => {
  assert.equal(process.env.SCHEDULER_ENABLED, 'false', 'run against the explicitly disabled Compose scheduler');
  const observer = await observe();
  try {
    const first = await http('/devices/dev-thermostat/telemetry?limit=2');
    assert.equal(first.items.length, 2, 'saved history is a prerequisite');
    for (let i = 0; i < 20; i++) {
      assert.deepEqual(await http('/devices/dev-thermostat/telemetry?limit=2'), first);
      assert.equal((await http('/devices/dev-thermostat')).id, 'dev-thermostat');
    }
    await delay(5000); // More than two normal 2-second periods, after explicit disable.
    assert.deepEqual(await http('/devices/dev-thermostat/telemetry?limit=2'), first);
    assert.equal(observer.events.filter(e => e.key === 'poll.requested').length, 0, JSON.stringify(observer.events));
    console.log(JSON.stringify({retained: first.items.map(m => ({event_id: m.event_id, measured_at: m.measured_at})), getRequests: 42, observedPolls: 0}));
  } finally { await observer.close(); }
});
