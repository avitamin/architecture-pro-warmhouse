import { cp, mkdir, readFile, writeFile, stat } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import SwaggerParser from '@apidevtools/swagger-parser';
import $RefParser from '@apidevtools/json-schema-ref-parser';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
if (!process.argv[2]) throw new Error('Usage: npm run build -- /absolute/new/output-directory');
const out = path.resolve(process.argv[2]);
try {
  await stat(out);
  throw new Error(`Output already exists: ${out}; choose a new directory`);
} catch (error) {
  if (error.code !== 'ENOENT') throw error;
}
await mkdir(out, { recursive: true });
await cp(path.join(root, 'viewer'), out, { recursive: true });
await mkdir(path.join(out, 'assets'));
for (const [source, target] of [
  ['swagger-ui-dist/swagger-ui.css', 'swagger-ui.css'],
  ['swagger-ui-dist/swagger-ui-bundle.js', 'swagger-ui-bundle.js'],
  ['swagger-ui-dist/LICENSE', 'swagger-ui.LICENSE'],
  ['@asyncapi/react-component/styles/default.min.css', 'asyncapi.css'],
  ['@asyncapi/react-component/browser/standalone/index.js', 'asyncapi.js'],
  ['@asyncapi/react-component/browser/standalone/index.js.LICENSE.txt', 'asyncapi.js.LICENSE.txt'],
  ['@asyncapi/react-component/LICENSE', 'asyncapi.LICENSE'],
]) await cp(path.join(root, 'node_modules', source), path.join(out, 'assets', target));
const openapi = await SwaggerParser.dereference(path.join(root, 'openapi.yaml'));
await writeFile(path.join(out, 'openapi.json'), JSON.stringify(openapi));
for (const app of ['telemetry', 'devices', 'scenarios', 'integration-gateway']) {
  const schema = await $RefParser.dereference(path.join(root, 'asyncapi', `${app}.yaml`));
  await writeFile(path.join(out, `${app}.json`), JSON.stringify(schema));
}
console.log(`Built local documentation: ${out}`);
