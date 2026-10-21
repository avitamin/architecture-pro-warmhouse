import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
import SwaggerParser from '@apidevtools/swagger-parser';
import { Parser } from '@asyncapi/parser';
import Ajv from 'ajv';
import addFormats from 'ajv-formats';
import YAML from 'yaml';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = async name => YAML.parse(await readFile(path.join(root, name), 'utf8'));
const schemas = await read('schemas.json');
const fixtures = await read('examples/fixtures.json');
const ajv = new Ajv({ allErrors: true, strict: false });
addFormats(ajv);
ajv.addSchema(schemas, 'shared');
let examples = 0;
function validate(name, payload, expected = true) {
  const check = ajv.compile({ $ref: `shared#/definitions/${name}` });
  const valid = check(payload);
  assert.equal(valid, expected, `${name}: ${JSON.stringify(check.errors)}`);
  examples++;
}
for (const [name, payload] of Object.entries(fixtures)) validate(name, payload);
const api = await SwaggerParser.validate(path.join(root, 'openapi.yaml'));
let operations = 0;
for (const item of Object.values(api.paths)) {
  for (const [method, operation] of Object.entries(item)) {
    if (!['get', 'post', 'put'].includes(method)) continue;
    operations++;
    assert.ok(operation.operationId);
    assert.ok(operation.responses);
    for (const parameter of operation.parameters ?? []) {
      assert.ok(Object.keys(parameter.examples).length, `${operation.operationId} parameter examples`);
      const check = ajv.compile(parameter.schema);
      for (const example of Object.values(parameter.examples)) {
        assert.ok(check(example.value), `${operation.operationId} ${parameter.name}: ${JSON.stringify(check.errors)}`);
        examples++;
      }
    }
    const contents = Object.values(operation.responses).map(r => r.content).filter(Boolean);
    if (operation.requestBody) contents.push(operation.requestBody.content);
    for (const content of contents) {
      const media = content['application/json'];
      assert.ok(Object.keys(media.examples).length, `${operation.operationId} missing examples`);
      const check = ajv.compile(media.schema);
      for (const example of Object.values(media.examples)) {
        assert.ok(check(example.value), `${operation.operationId}: ${JSON.stringify(check.errors)}`);
        examples++;
      }
    }
  }
}
assert.equal(operations, 8);
const errorCodes = {400: 'INVALID_ARGUMENT', 401: 'UNAUTHENTICATED', 403: 'PERMISSION_DENIED',
  404: 'NOT_FOUND', 409: 'CONFLICT', 422: 'VALIDATION_FAILED', 500: 'INTERNAL',
  503: 'UNAVAILABLE', 504: 'DEADLINE_EXCEEDED'};
for (const item of Object.values(api.paths)) {
  for (const [method, operation] of Object.entries(item)) {
    if (!['get', 'post', 'put'].includes(method)) continue;
    for (const [status, response] of Object.entries(operation.responses)) {
      if (!errorCodes[status]) continue;
      for (const example of Object.values(response.content['application/json'].examples)) {
        assert.equal(example.value.code, errorCodes[status], `${operation.operationId} HTTP ${status}`);
      }
    }
  }
}
const responseExample = (operation, status) => operation.responses[status].content['application/json'].examples.normal.value;
const requestExample = operation => operation.requestBody.content['application/json'].examples.normal.value;
const create = api.paths['/automation-rules'].post;
const created = responseExample(create, '201');
assert.equal(created.version, 1);
assert.equal(Object.hasOwn(created, 'activated_at'), created.enabled);
for (const [key, value] of Object.entries(requestExample(create))) assert.deepEqual(created[key], value, `createRule ${key}`);
assert.equal(create.responses['409'], undefined);
const update = api.paths['/automation-rules/{rule_id}'].put;
const updated = responseExample(update, '200');
assert.equal(updated.version, requestExample(update).expected_version + 1);
for (const [key, value] of Object.entries(requestExample(update))) {
  if (key !== 'expected_version') assert.deepEqual(updated[key], value, `updateRule ${key}`);
}
assert.equal(updated.activated_at, created.activated_at, 'disable preserves activation diagnostics');
console.log(`PASS OpenAPI 3.1.1: ${operations} operations`);
const parser = new Parser();
for (const app of ['telemetry', 'devices', 'scenarios', 'integration-gateway']) {
  const source = path.join(root, 'asyncapi', `${app}.yaml`);
  const { document, diagnostics } = await parser.parse(await readFile(source, 'utf8'), { source });
  const errors = diagnostics.filter(d => d.severity === 0);
  assert.equal(errors.length, 0, `${app}: ${JSON.stringify(errors)}`);
  assert.ok(document, `${app}: no parsed document`);
  console.log(`PASS AsyncAPI 3.0.0: ${app} (${diagnostics.length} non-error diagnostics)`);
}
const messages = await read('asyncapi/messages.yaml');
for (const [name, message] of Object.entries(messages.components.messages)) {
  for (const ex of message.examples) validate(name, ex.payload);
}
const resultSnapshots = new Map();
for (const { payload } of messages.components.messages.CommandStateChanged.examples) {
  const key = `${payload.command_id}:${payload.result_version}`;
  if (resultSnapshots.has(key)) assert.deepEqual(payload, resultSnapshots.get(key), `Conflicting result ${key}`);
  else resultSnapshots.set(key, payload);
}
const clone = name => structuredClone(fixtures[name]);
const missingId = clone('MeasurementRecorded'); delete missingId.event_id;
validate('MeasurementRecorded', missingId, false);
validate('MeasurementRecorded', { ...clone('MeasurementRecorded'), value: null }, false);
validate('CommandInput', { ...clone('CommandInput'), source: 'scenario' }, false);
validate('CommandStateChanged', { ...clone('CommandStateChanged'), status: 'accepted' }, false);
const missingVersion = clone('CommandStateChanged'); delete missingVersion.result_version;
validate('CommandStateChanged', missingVersion, false);
const missingLink = clone('PollCompleted'); delete missingLink.measurement_event_id;
validate('PollCompleted', missingLink, false);
validate('PollCompleted', { ...clone('PollCompleted'), outcome: 'timeout' }, false);
validate('RuleInput', { ...clone('RuleInput'), max_data_age_ms: 0 }, false);
validate('CommandStateChanged', { ...clone('CommandStateChanged'), result_version: 3 }, false);
validate('CommandInput', { ...clone('CommandInput'), expires_at: '2026-10-21T10:00:30+03:00' }, false);
validate('CommandStateChanged', { ...clone('CommandStateChanged'), reason_code: '' }, false);
validate('CommandStateChanged', { ...clone('CommandStateChanged'), status: 'queued', reason_code: 'COMPLETED' }, false);
console.log(`PASS ${examples} positive/negative schema example checks`);

// Проверяются независимые артефакты, а не имитация реализации сервисов.
const f = fixtures;
const manual = messages.components.messages.CommandRequested.examples.find(e => e.name === 'manual').payload;
for (const key of ['command_id', 'capability_id', 'parameters', 'expires_at']) {
  assert.deepEqual(manual[key], f.CommandInput[key], `manual input ${key}`);
}
for (const key of ['command_id', 'device_id', 'capability_id', 'user_id', 'source', 'parameters', 'expires_at', 'acceptance_seq']) {
  assert.deepEqual(manual[key], f.Command[key], `manual command ${key}`);
}
assert.equal(manual.manual_cancel_before_seq, manual.acceptance_seq);
for (const example of messages.components.messages.CommandRequested.examples) {
  const command = example.payload;
  if (command.source === 'scenario') {
    assert.equal(Date.parse(command.expires_at), Date.parse(f.TriggerPage.items[0].triggered_at) + f.Rule.command_ttl_ms);
  }
}
assert.equal(f.PollRequested.poll_job_id, f.PollCompleted.poll_job_id);
assert.equal(f.PollCompleted.measurement_event_id, f.MeasurementRecorded.event_id);
assert.equal(f.TriggerPage.items[0].event_id, f.MeasurementRecorded.event_id);
assert.equal(f.TriggerPage.items[0].command_id, f.CommandRequested.command_id);
assert.equal(f.CommandRequested.command_id, f.CommandStateChanged.command_id);
assert.equal(f.CommandRequested.rule_version, f.Rule.version);
for (const job of [f.PollRequested, f.CommandRequested]) {
  assert.equal(job.device_id, job.config.device_id);
  assert.equal(job.capability_id, job.config.capability_id);
  assert.ok(Date.parse(job.expires_at) <= Date.parse(job.config.valid_until));
}
assert.ok(Date.parse(f.PollRequested.occurred_at) <= Date.parse(f.MeasurementRecorded.measured_at));
assert.ok(Date.parse(f.MeasurementRecorded.measured_at) > Date.parse(f.Rule.activated_at));
assert.ok(Date.parse(f.CommandRequested.occurred_at) >= Date.parse(f.TriggerPage.items[0].triggered_at));
assert.ok(Date.parse(f.CommandStateChanged.occurred_at) >= Date.parse(f.CommandStateChanged.state_changed_at));
assert.equal(f.CommandStateChanged.evidence, 'operation_completed');
const capability = f.Device.capabilities.find(c => c.id === f.CommandInput.capability_id);
assert.ok(ajv.compile(capability.parameters_schema)(f.CommandInput.parameters));

const proto = await readFile(path.join(root, 'proto/warmhouse.proto'), 'utf8');
const shared = ['Parameters','Capability','Device','ConfigSnapshot','CommandInput','Command','Measurement','TelemetryPage','RuleInput','Trigger','TriggerPage'];
for (const name of shared) {
  const body = proto.match(new RegExp(`message ${name} \\{([\\s\\S]*?)\\n\\}`))?.[1];
  assert.ok(body, `Missing proto message ${name}`);
  const fields = [...body.matchAll(/\b(\w+)\s*=\s*\d+\s*;/g)].map(m => m[1]).sort();
  assert.deepEqual(fields, Object.keys(schemas.definitions[name].properties).sort(), `${name} field drift`);
}
assert.equal([...proto.matchAll(/\brpc \w+\(/g)].length, 16);
console.log('PASS cross-transport fields and flow fixture invariants; runtime services NOT RUN');
