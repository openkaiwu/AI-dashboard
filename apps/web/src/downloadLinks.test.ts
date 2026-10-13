import { expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

const releasePage = readFileSync(new URL('../../../deploy/downloads-v1.1.1.html', import.meta.url), 'utf8');
const configPage = readFileSync(new URL('../../../deploy/downloads-index.html', import.meta.url), 'utf8');

it('opens the current release page from the download-only port instead of the outdated site root', () => {
  const serverConfig = readFileSync(new URL('../../../deploy/nginx-aihub.conf', import.meta.url), 'utf8');
  const portBlock = serverConfig.match(/listen 8888;([\s\S]*?)\n}/)?.[1];
  expect(portBlock).toContain('return 302 /downloads/index.html;');
  expect(portBlock).toContain('alias /var/www/aihub-downloads/index.html;');
  expect(portBlock).not.toContain('root /var/www/aihub-downloads;');
});

it.each([
  ['release download page', releasePage],
  ['standard deployment download page', configPage],
])('%s routes its file links through port 8888', (_name, html) => {
  const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  expect(script).toBeTruthy();
  const paths = [
    '/downloads/AIHub-%E6%AD%A3%E5%BC%8F%E7%89%88v1.1.1.exe',
    '/downloads/AIHub-%E6%AD%A3%E5%BC%8F%E7%89%88v1.1.1.apk',
    '/downloads/AIHub-Codex-Collector-Windows-latest.zip',
    '/downloads/SHA256SUMS-v1.1.1.txt',
  ];
  const links = paths.map(href => ({ href, getAttribute: () => href }));
  runInNewContext(script!, {
    window: { location: { href: 'https://aihub.example.test/downloads/index.html' } },
    document: { querySelectorAll: () => links },
    URL,
  });
  expect(links.map(link => link.href)).toEqual([
    'http://aihub.example.test:8888/downloads/AIHub-%E6%AD%A3%E5%BC%8F%E7%89%88v1.1.1.exe',
    'http://aihub.example.test:8888/downloads/AIHub-%E6%AD%A3%E5%BC%8F%E7%89%88v1.1.1.apk',
    'http://aihub.example.test:8888/downloads/AIHub-Codex-Collector-Windows-latest.zip',
    'http://aihub.example.test:8888/downloads/SHA256SUMS-v1.1.1.txt',
  ]);
});
