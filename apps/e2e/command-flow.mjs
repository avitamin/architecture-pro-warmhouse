import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import Ajv from 'ajv';
import addFormats from 'ajv-formats';
import {observe, waitFor, delay} from './observer.mjs';

const schemas = JSON.parse(await readFile(new URL('./schemas.json', import.meta.url)));
const ajv = new Ajv({allErrors: true, strict: false});
addFormats(ajv); ajv.addSchema(schemas, 'contracts');
const validators = new Map(['CommandInput', 'Command', 'CommandRequested', 'CommandStateChanged'].map(name => [name, ajv.compile({$ref: `contracts#/definitions/${name}`})]));
function valid(name, body) { const check = validators.get(name); assert.ok(check(body), `${name}: ${JSON.stringify(check.errors)} payload=${JSON.stringify(body)}`); }
function instant(text) {
  assert.match(text, /Z$/);
  const [seconds, fraction = ''] = text.slice(0, -1).split('.');
  return BigInt(Date.parse(seconds + 'Z')) * 1000000n + BigInt(fraction.padEnd(9, '0'));
}

export async function executeCommand(enabled) {
  const observer = await observe([['wh.jobs.v1', 'command.requested'], ['wh.events.v1', 'command.state_changed']]);
  const command_id = randomUUID();
  const path = '/api/v1/devices/dev-thermostat/commands';
  const input = {command_id, capability_id: 'cap-heating', parameters: {operation: 'set', arguments: {enabled}}, expires_at: new Date(Date.now() + 60000).toISOString()};
  valid('CommandInput', input);
  try {
    const post = await fetch(process.env.BASE_URL + path, {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(input), signal: AbortSignal.timeout(5000)});
    const text = await post.text();
    assert.equal(post.status, 202, `command_id=${command_id}: ${text}`);
    assert.equal(post.headers.get('location'), `${path}/${command_id}`);
    const accepted = JSON.parse(text); valid('Command', accepted);
    assert.equal(accepted.status, 'accepted');
    assert.equal('result_version' in accepted, false);
    assert.equal('reason_code' in accepted, false);
    let current;
    const deadline = Date.now() + 10000;
    do {
      const get = await fetch(process.env.BASE_URL + post.headers.get('location'), {signal: AbortSignal.timeout(3000)});
      assert.equal(get.status, 200, `GET command_id=${command_id}`);
      current = await get.json();
      if (current.status === 'succeeded') break;
      await delay(50);
    } while (Date.now() < deadline);
    assert.equal(current.status, 'succeeded', `command_id=${command_id}: ${JSON.stringify(current)}`);
    valid('Command', current);
    for (const snapshot of [accepted, current]) {
      assert.equal(snapshot.command_id, command_id);
      assert.equal(snapshot.device_id, 'dev-thermostat');
      assert.equal(snapshot.capability_id, 'cap-heating');
      assert.equal(snapshot.user_id, 'user-1');
      assert.equal(snapshot.source, 'manual');
      assert.deepEqual(snapshot.parameters, input.parameters);
      assert.equal(instant(snapshot.expires_at), instant(input.expires_at));
      assert.ok(BigInt(snapshot.acceptance_seq) > 0n);
      for (const absent of ['rule_id', 'rule_version', 'trigger_id', 'replayed', 'command']) assert.equal(absent in snapshot, false);
    }
    const diagnostic = await fetch(process.env.TEMPERATURE_URL + '/diagnostics/heating', {signal: AbortSignal.timeout(3000)});
    assert.equal(diagnostic.status, 200);
    const actual = await diagnostic.json();
    assert.deepEqual(actual, {enabled, last_command_id: command_id});
    const job = await waitFor(() => observer.events.find(e => e.key === 'command.requested' && e.body.command_id === command_id), `CommandRequested ${command_id}`);
    const result = await waitFor(() => observer.events.find(e => e.key === 'command.state_changed' && e.body.command_id === command_id), `CommandStateChanged ${command_id}`);
    for (const [event, schema] of [[job, 'CommandRequested'], [result, 'CommandStateChanged']]) {
      valid(schema, event.body);
      assert.equal(event.properties.deliveryMode, 2);
      assert.equal(event.properties.contentType, 'application/json');
      assert.equal(event.body.type, event.key);
      assert.equal(event.body.schema_version, 1);
      assert.equal(event.body.device_id, 'dev-thermostat');
      assert.equal(event.body.capability_id, 'cap-heating');
    }
    const j = job.body, r = result.body;
    assert.notEqual(j.event_id, r.event_id);
    assert.equal(j.source, 'manual'); assert.equal(j.user_id, 'user-1');
    assert.deepEqual(j.parameters, input.parameters); // Explicitly retains boolean false on the wire.
    assert.equal(j.acceptance_seq, accepted.acceptance_seq);
    assert.equal(j.manual_cancel_before_seq, j.acceptance_seq);
    assert.equal(instant(j.expires_at), instant(input.expires_at));
    assert.deepEqual({device_id: j.config.device_id, capability_id: j.config.capability_id, config_version: j.config.config_version,
      adapter_key: j.config.adapter_key, endpoint_key: j.config.endpoint_key, channel: j.config.channel},
    {device_id: 'dev-thermostat', capability_id: 'cap-heating', config_version: 1, adapter_key: 'partner-thermostat', endpoint_key: 'registered-endpoint-1', channel: 'heating'});
    assert.ok(instant(j.config.issued_at) <= instant(j.occurred_at));
    assert.ok(instant(j.occurred_at) < instant(j.expires_at));
    assert.ok(instant(j.expires_at) <= instant(j.config.valid_until));
    assert.equal(r.status, 'succeeded'); assert.equal(r.evidence, 'operation_completed'); assert.equal(r.reason_code, 'COMPLETED');
    assert.equal(r.result_version, '1'); assert.equal(current.result_version, r.result_version);
    assert.equal(current.reason_code, r.reason_code);
    assert.equal(current.acceptance_seq, accepted.acceptance_seq);
    assert.equal(current.created_at, accepted.created_at);
    assert.equal(instant(current.state_changed_at), instant(r.state_changed_at));
    assert.equal(instant(accepted.state_changed_at), instant(accepted.created_at));
    assert.ok(instant(accepted.created_at) <= instant(r.state_changed_at));
    assert.ok(instant(r.state_changed_at) <= instant(r.occurred_at));
    assert.ok(instant(r.state_changed_at) < instant(input.expires_at));
    console.log(JSON.stringify({command_id, enabled, accepted, current, diagnostic: actual, requested: j, result: r}));
    return current;
  } finally { await observer.close(); }
}
