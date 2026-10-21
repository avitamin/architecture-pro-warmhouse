import test from 'node:test';
import assert from 'node:assert/strict';
import {executeCommand} from './command-flow.mjs';

test('real on/off command payloads conform to current schemas and correlate with execution', {timeout: 45000}, async () => {
  const on = await executeCommand(true);
  const off = await executeCommand(false);
  assert.notEqual(on.command_id, off.command_id);
  assert.ok(BigInt(off.acceptance_seq) > BigInt(on.acceptance_seq));
});
