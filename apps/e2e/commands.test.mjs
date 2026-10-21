import test from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {delay} from './observer.mjs';
import {executeCommand} from './command-flow.mjs';
let enabledCommandID;

test('manual heating command is accepted then actually enabled in the simulator', {timeout: 20000}, async () => {
  const command_id = randomUUID();
  enabledCommandID = command_id;
  const path = `/api/v1/devices/dev-thermostat/commands`;
  const input = {command_id, capability_id: 'cap-heating', parameters: {operation: 'set', arguments: {enabled: true}}, expires_at: new Date(Date.now() + 60000).toISOString()};
  const post = await fetch(process.env.BASE_URL + path, {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(input), signal: AbortSignal.timeout(5000)});
  const text = await post.text();
  assert.equal(post.status, 202, text);
  assert.equal(post.headers.get('location'), `${path}/${command_id}`);
  const accepted = JSON.parse(text);
  assert.equal(accepted.command_id, command_id);
  assert.equal(accepted.status, 'accepted');
  let current;
  const deadline = Date.now() + 10000;
  do {
    const get = await fetch(process.env.BASE_URL + post.headers.get('location'), {signal: AbortSignal.timeout(3000)});
    assert.equal(get.status, 200);
    current = await get.json();
    if (current.status === 'succeeded') break;
    await delay(50);
  } while (Date.now() < deadline);
  assert.equal(current.status, 'succeeded', JSON.stringify(current));
  assert.equal(current.command_id, command_id);
  assert.equal(current.parameters.arguments.enabled, true);
  const diagnostic = await fetch(process.env.TEMPERATURE_URL + '/diagnostics/heating', {signal: AbortSignal.timeout(3000)});
  assert.equal(diagnostic.status, 200);
  assert.deepEqual(await diagnostic.json(), {enabled: true, last_command_id: command_id});
  console.log(JSON.stringify({accepted, current}));
});

test('a separate new manual command switches heating off and retains false across HTTP, protobuf and JSON', {timeout: 25000}, async () => {
  const off = await executeCommand(false);
  assert.notEqual(off.command_id, enabledCommandID);
});
