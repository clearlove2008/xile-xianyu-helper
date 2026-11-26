import { describe, expect, it } from 'vitest';
import { netdiskPlatformLabel } from './types';

describe('netdisk platform labels', () => {
  it('maps known platforms to Chinese labels', () => {
    expect(netdiskPlatformLabel('quark')).toBe('夸克网盘');
    expect(netdiskPlatformLabel('baidu')).toBe('百度网盘');
  });

  it('passes through unknown platforms', () => {
    expect(netdiskPlatformLabel('xunlei')).toBe('xunlei');
  });
});
