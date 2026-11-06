const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');

const html = readFileSync(`${__dirname}/templates/index.html`, 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
function element() {
  return {
    value: '', textContent: '', hidden: false, disabled: false,
    children: [], listeners: {}, classList: { add() {}, remove() {} },
    replaceChildren() { this.children = []; },
    append(...items) { this.children.push(...items); },
    addEventListener(event, handler) { this.listeners[event] = handler; },
    setAttribute() {}, focus() {},
  };
}
const elements = new Map();
let copied;
let navigated;
const context = vm.createContext({
  URL, setTimeout() {},
  navigator: { clipboard: { async writeText(text) { copied = text; } } },
  window: { isSecureContext: true, location: { origin: 'https://proxy.test', assign(url) { navigated = url; } } },
  document: {
    getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    querySelectorAll() { return []; },
    createElement: element,
  },
});
vm.runInContext(script, context);
const classify = value => context.classifyInput(value, 'https://proxy.test');

test('repository forms generate a clone command on the current deployment', () => {
  for (const value of [
    'https://github.com/user/repo', 'https://github.com/user/repo.git/',
    'github.com/user/repo', 'user/repo', 'git@github.com:user/repo.git',
    'https://proxy.test/user/repo', 'https://proxy.test/https://github.com/user/repo',
    'https://proxy.test/github.com/user/repo', 'https://github.com/user/repo?tab=readme#readme',
  ]) {
    const result = classify(value);
    assert.equal(result.repository, true, value);
    assert.equal(result.proxyURL, 'https://proxy.test/user/repo.git', value);
    assert.equal(result.commands[0][1], "git clone 'https://proxy.test/user/repo.git'", value);
  }
});

test('files produce wget and curl commands, preserving escaped paths and query strings', () => {
  for (const path of [
    'releases/download/v1/app.zip', 'releases/latest/download/app.zip',
    'archive/refs/heads/main.zip', 'blob/main/a%20b.txt', 'raw/main/README.md',
  ]) {
    const result = classify('https://github.com/user/repo/' + path + '?raw=1&download=1#L1');
    assert.equal(result.repository, false);
    assert.equal(result.proxyURL, 'https://proxy.test/user/repo/' + path + '?raw=1&download=1');
    assert.match(result.commands[0][1], /^wget -O /);
    assert.match(result.commands[1][1], /^curl -fL -o /);
  }
  for (const url of [
    'https://raw.githubusercontent.com/user/repo/main/file?token=abc',
    'https://gist.githubusercontent.com/user/id/raw/hash/file',
    'https://raw.github.com/user/repo/main/file',
  ]) {
    assert.equal(classify(url).proxyURL, 'https://proxy.test/' + url);
    assert.equal(classify('https://proxy.test/' + url).proxyURL, 'https://proxy.test/' + url);
  }
});

test('invalid inputs and GitHub web pages do not produce commands', () => {
  assert.equal(classify('  '), null);
  for (const value of [
    'https://evil.test/user/repo', 'https://github.com.evil.test/user/repo',
    'https://github.com@evil.test/user/repo', 'https://github.com:8443/user/repo',
    'https://github.com/user/repo/issues', 'https://github.com/user/repo/tree/main',
    'https://github.com/user/repo/releases/tag/v1', 'https://github.com/user/repo/blob/main',
    'https://github.com/user//repo', 'not a url',
  ]) assert.throws(() => classify(value), undefined, value);
});

test('shell metacharacters remain inside quoted arguments', () => {
  const result = classify("https://github.com/user/repo/raw/main/$(whoami)'file.txt?name=`id`");
  for (const [, command] of result.commands) {
    assert.equal((command.match(/'/g) || []).length, 4);
    assert.ok(command.includes('%27'));
  }
});

test('input updates commands immediately and submission copies or downloads by type', async () => {
  const input = elements.get('url');
  const submit = elements.get('submit');
  const form = elements.get('proxy-form');
  assert.equal(elements.get('result').hidden, true);
  input.value = 'https://github.com/user/repo';
  input.listeners.input();
  assert.equal(elements.get('result').hidden, false);
  assert.equal(elements.get('commands').children.length, 1);
  assert.equal(submit.textContent, '复制克隆命令');
  form.listeners.submit({ preventDefault() {} });
  await Promise.resolve();
  assert.equal(copied, "git clone 'https://proxy.test/user/repo.git'");
  assert.equal(navigated, undefined);
  input.value = 'https://github.com/user/repo/blob/main/file.txt';
  input.listeners.input();
  assert.equal(elements.get('commands').children.length, 2);
  assert.equal(submit.textContent, '下载文件');
  form.listeners.submit({ preventDefault() {} });
  assert.equal(navigated, 'https://proxy.test/user/repo/blob/main/file.txt');
  input.value = 'https://example.com/unsupported';
  input.listeners.input();
  assert.equal(submit.disabled, true);
  assert.equal(elements.get('result').hidden, true);
});
