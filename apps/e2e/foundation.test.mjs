import test from 'node:test';
import assert from 'node:assert/strict';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import amqp from 'amqplib';

test('runner validates date-time and connects to the real internal vhost', {timeout: 15000}, async () => {
  const ajv = new Ajv({allErrors: true});
  addFormats(ajv);
  const valid = ajv.compile({type: 'string', format: 'date-time'});
  assert.equal(valid('invalid-time'), false);
  assert.equal(valid('2026-02-30T00:00:00Z'), false);
  assert.equal(valid(new Date().toISOString()), true);
  const connection = await amqp.connect(process.env.AMQP_URL, {timeout: 5000});
  try {
    const channel = await connection.createChannel();
    await channel.close();
  } finally { await connection.close(); }
});
