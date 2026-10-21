import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import Ajv from 'ajv';
import addFormats from 'ajv-formats';
import {observe, waitFor} from './observer.mjs';

test('real AMQP poll, measurement and completion conform to existing schemas and correlate', {timeout: 25000}, async () => {
  const schemas = JSON.parse(await readFile(new URL('./schemas.json', import.meta.url)));
  const ajv = new Ajv({allErrors: true, strict: false});
  addFormats(ajv);
  ajv.addSchema(schemas, 'contracts');
  const validators = new Map([['poll.requested', 'PollRequested'], ['measurement.recorded', 'MeasurementRecorded'], ['poll.completed', 'PollCompleted']].map(([key, name]) => [key, ajv.compile({$ref: `contracts#/definitions/${name}`})]));
  const observer = await observe(); // Subscription is ready before the next background poll.
  try {
    const job = await waitFor(() => observer.events.find(e => e.key === 'poll.requested')?.body, 'new PollRequested');
    const measurement = await waitFor(() => observer.events.find(e => e.key === 'measurement.recorded' && e.body.poll_job_id === job.poll_job_id), `measurement for ${job.poll_job_id}`);
    const result = await waitFor(() => observer.events.find(e => e.key === 'poll.completed' && e.body.poll_job_id === job.poll_job_id), `completion for ${job.poll_job_id}`);
    for (const event of observer.events.filter(e => e.body.poll_job_id === job.poll_job_id)) {
      const valid = validators.get(event.key);
      assert.ok(valid(event.body), JSON.stringify(valid.errors));
      assert.equal(event.properties.deliveryMode, 2);
      assert.equal(event.properties.contentType, 'application/json');
      assert.equal(event.body.type, event.key);
      assert.equal(event.body.device_id, 'dev-thermostat');
      assert.equal(event.body.capability_id, 'cap-temperature');
    }
    assert.equal(measurement.body.value, Number(process.env.EXPECTED_TEMPERATURE ?? 17));
    assert.equal(measurement.body.unit, 'Cel');
    assert.equal(result.body.outcome, 'succeeded');
    assert.equal(result.body.measurement_event_id, measurement.body.event_id);
    assert.equal(new Set([job.event_id, measurement.body.event_id, result.body.event_id]).size, 3);
    assert.equal(job.config.device_id, job.device_id);
    assert.equal(job.config.capability_id, job.capability_id);
    assert.equal(job.config.adapter_key, 'partner-thermostat');
    assert.equal(job.config.endpoint_key, 'registered-endpoint-1');
    assert.equal(job.config.channel, 'temperature');
    assert.ok(Date.parse(job.config.issued_at) <= Date.parse(job.occurred_at));
    assert.ok(Date.parse(job.expires_at) > Date.parse(job.occurred_at));
    assert.ok(Date.parse(job.expires_at) <= Date.parse(job.config.valid_until));
    assert.ok(Date.parse(measurement.body.measured_at) <= Date.parse(measurement.body.gateway_received_at));
    assert.ok(Date.parse(measurement.body.gateway_received_at) <= Date.parse(measurement.body.occurred_at));
    console.log(JSON.stringify({poll: job.poll_job_id, measurement: measurement.body.event_id, completed: result.body.event_id, value: measurement.body.value}));
  } finally { await observer.close(); }
});
