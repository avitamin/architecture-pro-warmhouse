import amqp from 'amqplib';
import assert from 'node:assert/strict';

export async function observe() {
  const connection = await amqp.connect(process.env.AMQP_URL, {timeout: 5000});
  const channel = await connection.createChannel();
  const events = [];
  const {queue} = await channel.assertQueue('', {exclusive: true, autoDelete: true});
  for (const [exchange, key] of [['wh.jobs.v1', 'poll.requested'], ['wh.events.v1', 'measurement.recorded'], ['wh.events.v1', 'poll.completed']]) {
    await channel.bindQueue(queue, exchange, key);
  }
  await channel.consume(queue, delivery => {
    if (delivery) events.push({key: delivery.fields.routingKey, properties: delivery.properties, body: JSON.parse(delivery.content.toString())});
  }, {noAck: true});
  return {events, close: () => connection.close()};
}
export const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
export async function waitFor(get, description, timeout = 15000) {
  const deadline = Date.now() + timeout;
  do { const value = get(); if (value) return value; await delay(50); } while (Date.now() < deadline);
  assert.fail(`deadline waiting for ${description}`);
}
export async function http(path) {
  const response = await fetch(`${process.env.BASE_URL}/api/v1${path}`, {signal: AbortSignal.timeout(5000)});
  const text = await response.text();
  assert.equal(response.status, 200, `${path}: ${response.status}: ${text}`);
  return JSON.parse(text);
}
