// Run with: node desktop_bridge_test.js
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const script = fs.readFileSync('web/index.html', 'utf8').split('<script>')[1].split('let themeValue')[0].replace('(()=>{', '');
function createContext(native, fetch) {
  const context = {document: {}, window: {}, setTimeout, clearTimeout, fetch};
  if (native) context.window.linkdropRequest = native(context);
  vm.createContext(context);
  vm.runInContext(script + '\nglobalThis.api=API;', context);
  return context;
}
(async () => {
  let call;
  const context = createContext(ctx => async (id, method, path, body) => {
    call = {method, path, body};
    setImmediate(() => ctx.window.linkdropRespond(id, {status: 200, body: {ready: true}}));
  }, () => {throw new Error('Desktop must not use fetch');});
  assert.equal((await context.api('status')).ready, true);
  assert.deepEqual(call, {method: 'GET', path: '/api/status', body: ''});
  await context.api('download', {url: 'https://example.com/media.mp4'});
  assert.equal(call.method, 'POST');
  assert.equal(call.path, '/api/download');
  assert.equal(JSON.parse(call.body).url, 'https://example.com/media.mp4');

  const denied = createContext(ctx => async id => {
    setImmediate(() => ctx.window.linkdropRespond(id, {status: 409, body: {error: 'Componentes no listos'}}));
  });
  await assert.rejects(denied.api('download', {}), /Componentes no listos/);
  const disconnected = createContext(() => async () => {throw new Error('Bridge disconnected');});
  await assert.rejects(disconnected.api('status'), /Bridge disconnected/);

  const browser = createContext(null, async (path, request) => {
    assert.equal(path, '/api/status');
    assert.equal(request.method, 'GET');
    return {ok: true, json: async () => ({ready: true})};
  });
  assert.equal((await browser.api('status')).ready, true);
  console.log('PASS: desktop bridge routing, payloads, API errors, connection errors and Linux HTTP fallback');
})().catch(error => {console.error(error); process.exitCode = 1;});
