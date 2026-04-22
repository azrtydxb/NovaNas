import { describe, expect, it } from 'vitest';
import { invalidationForChannel } from './query-invalidation';

describe('invalidationForChannel', () => {
  it('maps pool channels', () => {
    expect(invalidationForChannel('pool:*')).toEqual([['pools']]);
    expect(invalidationForChannel('pool:fast')).toEqual([['pools'], ['pool', 'fast']]);
  });

  it('maps dataset channels', () => {
    expect(invalidationForChannel('dataset:*')).toEqual([['datasets']]);
    expect(invalidationForChannel('dataset:tank/home')).toEqual([
      ['datasets'],
      ['dataset', 'tank/home'],
    ]);
  });

  it('maps disk channels', () => {
    expect(invalidationForChannel('disk:0x5000c500abc')).toEqual([
      ['disks'],
      ['disk', '0x5000c500abc'],
    ]);
  });

  it('maps share, bucket, snapshot channels', () => {
    expect(invalidationForChannel('share:home')).toEqual([['shares'], ['share', 'home']]);
    expect(invalidationForChannel('bucket:media')).toEqual([['buckets'], ['bucket', 'media']]);
    expect(invalidationForChannel('snapshot:s1')).toEqual([['snapshots'], ['snapshot', 's1']]);
  });

  it('maps namespaced vm channels', () => {
    expect(invalidationForChannel('vm:default/web1')).toEqual([
      ['vms'],
      ['vm', 'default', 'web1'],
      ['vm', 'web1'],
    ]);
    expect(invalidationForChannel('vm:*')).toEqual([['vms']]);
  });

  it('maps namespaced app-instance channels', () => {
    expect(invalidationForChannel('appinstance:default/nextcloud')).toEqual([
      ['app-instances'],
      ['app-instance', 'default', 'nextcloud'],
      ['app-instance', 'nextcloud'],
    ]);
  });

  it('maps job, alert, system channels', () => {
    expect(invalidationForChannel('job:abc')).toEqual([['jobs'], ['job', 'abc']]);
    expect(invalidationForChannel('alert:critical')).toEqual([['alerts'], ['alerts', 'critical']]);
    expect(invalidationForChannel('system:*')).toEqual([['system']]);
  });

  it('returns empty for unknown channels', () => {
    expect(invalidationForChannel('nope:x')).toEqual([]);
    expect(invalidationForChannel('')).toEqual([]);
  });
});
