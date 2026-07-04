// Bundles the extension entry (src/extension.ts) into a single CommonJS file
// (dist/extension.js) that VS Code loads. `vscode` is provided by the host at
// runtime, so it is marked external. Run via `npm run build` / `npm run watch`.
import esbuild from 'esbuild';

const watch = process.argv.includes('--watch');

const options = {
  entryPoints: ['src/extension.ts'],
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node18',
  outfile: 'dist/extension.js',
  external: ['vscode'],
  sourcemap: true,
  logLevel: 'info',
};

if (watch) {
  const ctx = await esbuild.context(options);
  await ctx.watch();
  console.log('[esbuild] watching…');
} else {
  await esbuild.build(options);
}
