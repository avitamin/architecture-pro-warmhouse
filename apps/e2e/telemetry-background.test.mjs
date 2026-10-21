import test from 'node:test';
import assert from 'node:assert/strict';
import {observe, delay, http} from './observer.mjs';

// Normalize RFC3339 fractional seconds without discarding nanoseconds.
function instant(text) {
  const [seconds, fraction = ''] = text.replace(/Z$/, '').split('.');
  return BigInt(Date.parse(`${seconds}Z`)) * 1000000n + BigInt(fraction.padEnd(9, '0'));
}
test('history grows without HTTP and retains event timestamps on repeated reads', {timeout: 25000}, async () => {
  const observer = await observe();
  try {
    const baseline = await http('/devices/dev-thermostat/telemetry?limit=1');
    assert.ok(baseline.items.length);
    const boundary = Date.now();
    await delay(6500); // No HTTP calls during more than three configured poll periods.
    const events = observer.events.filter(e => e.key === 'measurement.recorded' && Date.parse(e.body.measured_at) > boundary);
    assert.ok(events.length >= 2, `background measurements: ${JSON.stringify(observer.events.map(e => e.body))}`);
    const history = await http('/devices/dev-thermostat/telemetry');
    for (const {body} of events) {
      const saved = history.items.find(m => m.event_id === body.event_id);
      assert.ok(saved, `missing saved event_id=${body.event_id}`);
      assert.equal(saved.value, body.value);
      assert.equal(instant(saved.measured_at), instant(body.measured_at), 'persisted measured_at must retain the event time');
      assert.equal(instant(saved.gateway_received_at), instant(body.gateway_received_at));
    }
    const item = history.items.find(m => m.id === baseline.items[0].id);
    assert.equal(item.measured_at, baseline.items[0].measured_at);
    assert.equal(item.gateway_received_at, baseline.items[0].gateway_received_at);
    const device = await http('/devices/dev-thermostat');
    assert.equal(device.id, 'dev-thermostat');
    assert.equal(device.house_id, 'house-home');
    assert.equal(device.capabilities.find(c => c.id === 'cap-temperature').unit, 'Cel');
  } finally { await observer.close(); }
});
