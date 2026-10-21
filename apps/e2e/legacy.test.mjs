import test from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';

const base = process.env.BASE_URL;
const temperature = process.env.TEMPERATURE_URL;
async function request(url, status = 200, options = {}) {
  const response = await fetch(url, {...options, signal: AbortSignal.timeout(5000)});
  const text = await response.text();
  assert.equal(response.status, status, `${url}: ${response.status}: ${text}`);
  return JSON.parse(text);
}
function reading(value, timestamp, before) {
  assert.ok(Number.isFinite(value) && value >= 15 && value <= 30, `value=${value}`);
  assert.ok(Math.abs(value * 10 - Math.round(value * 10)) < 1e-8);
  assert.match(timestamp, /Z$/);
  assert.ok(Date.parse(timestamp) >= before && Date.parse(timestamp) <= Date.now(), timestamp);
}

test('real Create Sensor → Get All Sensors → Get by ID with fresh variable readings', {timeout: 30000}, async () => {
  const name = `stage1-${randomUUID()}`;
  const created = await request(`${base}/api/v1/sensors`, 201, {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({name, type: 'temperature', location: 'Living Room', unit: '°C'})
  });
  assert.ok(Number.isInteger(created.id) && created.id > 0);
  assert.equal(created.name, name);
  assert.equal(created.value, 0);
  assert.equal(created.status, 'inactive');
  let previous;
  let previousTime;
  const values = [];
  for (let i = 0; i < 5; i++) {
    const before = Date.now();
    const sensors = await request(`${base}/api/v1/sensors`);
    assert.ok(Array.isArray(sensors));
    const sensor = sensors.find(s => s.id === created.id);
    assert.ok(sensor, `missing id=${created.id}`);
    assert.equal(sensor.name, name);
    assert.equal(sensor.status, 'active');
    reading(sensor.value, sensor.last_updated, before);
    assert.notEqual(sensor.value, previous);
    if (previousTime) assert.ok(Date.parse(sensor.last_updated) > Date.parse(previousTime));
    previous = sensor.value;
    previousTime = sensor.last_updated;
    values.push(sensor.value);
  }
  const before = Date.now();
  const sensor = await request(`${base}/api/v1/sensors/${created.id}`);
  assert.equal(sensor.id, created.id);
  assert.equal(sensor.name, name);
  assert.equal(sensor.status, 'active');
  reading(sensor.value, sensor.last_updated, before);
  assert.notEqual(sensor.value, previous);
  console.log(JSON.stringify({id: sensor.id, values, byID: sensor.value, last_updated: sensor.last_updated}));
});

test('both simulator HTTP routes preserve mapping, time and changing values', {timeout: 15000}, async () => {
  let previous;
  for (const path of ['/temperature?location=Living%20Room', '/temperature/1', '/temperature?location=Living%20Room']) {
    const before = Date.now();
    const result = await request(`${temperature}${path}`);
    reading(result.value, result.timestamp, before);
    assert.equal(result.sensor_id, '1');
    assert.equal(result.location, 'Living Room');
    assert.equal(result.unit, '°C');
    assert.equal(result.status, 'active');
    assert.equal(result.sensor_type, 'temperature');
    assert.ok(result.description.length > 0);
    assert.notEqual(result.value, previous);
    previous = result.value;
  }
});
