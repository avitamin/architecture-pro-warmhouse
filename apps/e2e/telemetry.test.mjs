import test from 'node:test';
import assert from 'node:assert/strict';
const base = process.env.BASE_URL;
test('background measurement is readable through the monolith', {timeout: 30000}, async () => {
  const health = await fetch(`${base}/health`);
  assert.equal(health.status, 200, 'running monolith is a prerequisite');
  const deadline = Date.now() + 20000;
  let page;
  do {
    const response = await fetch(`${base}/api/v1/devices/dev-thermostat/telemetry`, {signal: AbortSignal.timeout(5000)});
    assert.equal(response.status, 200, `telemetry HTTP status: ${await response.clone().text()}`);
    page = await response.json();
    if (page.items.length) break;
    await new Promise(resolve => setTimeout(resolve, 250));
  } while (Date.now() < deadline);
  assert.ok(page.items.length > 0, 'background poll must produce history');
  const measurement = page.items[0];
  assert.equal(measurement.device_id, 'dev-thermostat');
  assert.equal(measurement.capability_id, 'cap-temperature');
  assert.equal(measurement.value, Number(process.env.EXPECTED_TEMPERATURE ?? 17));
  assert.equal(measurement.unit, 'Cel');
  for (const field of ['measured_at', 'gateway_received_at']) {
    assert.match(measurement[field], /Z$/);
    assert.ok(Number.isFinite(Date.parse(measurement[field])));
  }
});

test('HTTP maps device metadata, filters and pagination to the public gRPC contracts', {timeout: 15000}, async () => {
  async function get(path) {
    const response = await fetch(`${base}/api/v1${path}`, {signal: AbortSignal.timeout(5000)});
    assert.equal(response.status, 200, await response.clone().text());
    return response.json();
  }
  const device = await get('/devices/dev-thermostat');
  assert.equal(device.id, 'dev-thermostat');
  assert.equal(device.house_id, 'house-home');
  assert.equal(device.registration_status, 'bound');
  assert.equal(device.capabilities.find(c => c.id === 'cap-temperature').value_type, 'number');
  assert.equal(device.capabilities.find(c => c.id === 'cap-heating').parameters_schema.properties.arguments.required[0], 'enabled');
  const first = await get('/devices/dev-thermostat/telemetry?capability_id=cap-temperature&limit=2');
  assert.equal(first.items.length, 2);
  assert.ok(first.next_cursor);
  const second = await get(`/devices/dev-thermostat/telemetry?capability_id=cap-temperature&limit=2&cursor=${encodeURIComponent(first.next_cursor)}`);
  assert.ok(second.items.length > 0);
  assert.ok(second.items.every(m => !first.items.some(n => n.id === m.id)));
  const m = first.items[0];
  const query = new URLSearchParams({capability_id: 'cap-temperature', from: m.measured_at, to: new Date(Date.parse(m.measured_at) + 1).toISOString()});
  const filtered = await get(`/devices/dev-thermostat/telemetry?${query}`);
  assert.equal(filtered.items.length, 1);
  assert.deepEqual(filtered.items[0], m);
  assert.equal('next_cursor' in filtered, false);
  assert.deepEqual(await get('/devices/dev-thermostat/telemetry?capability_id=cap-heating'), {items: []});
});

test('HTTP exposes contract error codes for invalid pagination and unknown devices', {timeout: 15000}, async () => {
  for (const [path, expectedStatus, code] of [
    ['/devices/dev-thermostat/telemetry?limit=501', 400, 'INVALID_ARGUMENT'],
    ['/devices/missing', 404, 'NOT_FOUND'],
    ['/devices/missing/telemetry', 404, 'NOT_FOUND']
  ]) {
    const response = await fetch(`${base}/api/v1${path}`, {signal: AbortSignal.timeout(5000)});
    assert.equal(response.status, expectedStatus);
    const error = await response.json();
    assert.equal(error.code, code);
    assert.equal(error.retryable, false);
    assert.equal(typeof error.message, 'string');
    assert.ok(error.request_id);
  }
});
