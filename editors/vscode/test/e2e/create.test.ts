// End-to-end tests for `pineCreate` using the REAL `pine` binary.
import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { spawnSync } from 'node:child_process';
import { pineCreate } from '../../src/create';

const PINE = process.env.PINE_BIN && fs.existsSync(process.env.PINE_BIN)
  ? process.env.PINE_BIN
  : path.resolve(__dirname, '../../../../../pine');

describe('e2e: pineCreate against the real pine binary', function () {
  this.timeout(15000);
  let root: string;

  before(function () {
    if (!fs.existsSync(PINE)) {
      this.skip();
    }
  });
  beforeEach(() => {
    root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pine-create-')));
    const r = spawnSync(PINE, ['init'], { cwd: root });
    if (r.status !== 0) {
      throw new Error(`pine init failed: ${r.stderr?.toString()}`);
    }
  });
  afterEach(() => {
    fs.rmSync(root, { recursive: true, force: true });
  });

  it('creates a bug and writes a matching ticket file', async () => {
    const res = await pineCreate(PINE, root, 'bug', 'e2e sample bug');
    assert.ok(res.id, 'should return an id');
    assert.strictEqual(res.title, 'e2e sample bug');
    assert.match(res.id, /^[A-Z]+-[0-9a-z]+$/, `unexpected id form: ${res.id}`);
    const files = fs.readdirSync(path.join(root, '.pine', 'tickets'));
    assert.ok(files.includes(`${res.id}.md`), `expected ${res.id}.md among ${files.join(', ')}`);
  });

  it('rejects on an unknown ticket type', async () => {
    await assert.rejects(() => pineCreate(PINE, root, 'nonsense', 'x'));
  });
});
