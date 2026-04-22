import type { NavItem } from '@/types';

export const ADMIN_NAV: NavItem[] = [
  { id: 'dashboard', label: 'Dashboard', to: '/dashboard', icon: 'dashboard' },
  {
    id: 'storage',
    label: 'Storage',
    to: '/storage',
    icon: 'storage',
    children: [
      { id: 'pools', label: 'Pools', to: '/storage/pools' },
      { id: 'datasets', label: 'Datasets', to: '/storage/datasets' },
      { id: 'disks', label: 'Disks', to: '/storage/disks' },
      { id: 'snapshots', label: 'Snapshots', to: '/storage/snapshots' },
    ],
  },
  {
    id: 'sharing',
    label: 'Sharing',
    to: '/sharing',
    icon: 'share',
    children: [
      { id: 'shares', label: 'Shares', to: '/sharing/shares' },
      { id: 'iscsi', label: 'iSCSI / NVMe-oF', to: '/sharing/iscsi' },
      { id: 's3', label: 'S3', to: '/sharing/s3' },
    ],
  },
  {
    id: 'data-protection',
    label: 'Data Protection',
    to: '/data-protection',
    icon: 'protect',
    children: [
      { id: 'schedules', label: 'Snapshot schedules', to: '/data-protection/schedules' },
      { id: 'replication', label: 'Replication', to: '/data-protection/replication' },
      { id: 'cloud-backup', label: 'Cloud Backup', to: '/data-protection/cloud-backup' },
    ],
  },
  { id: 'apps', label: 'Apps', to: '/apps', icon: 'app' },
  { id: 'vms', label: 'Virtual Machines', to: '/vms', icon: 'vm' },
  { id: 'network', label: 'Network', to: '/network', icon: 'network' },
  {
    id: 'identity',
    label: 'Identity',
    to: '/identity',
    icon: 'identity',
    children: [
      { id: 'users', label: 'Users', to: '/identity/users' },
      { id: 'groups', label: 'Groups', to: '/identity/groups' },
    ],
  },
  {
    id: 'system',
    label: 'System',
    to: '/system',
    icon: 'system',
    children: [
      { id: 'settings', label: 'Settings', to: '/system/settings' },
      { id: 'updates', label: 'Updates', to: '/system/updates' },
      { id: 'certificates', label: 'Certificates', to: '/system/certificates' },
      { id: 'alerts', label: 'Alerts', to: '/system/alerts' },
      { id: 'audit', label: 'Audit log', to: '/system/audit' },
      { id: 'support', label: 'Support', to: '/system/support' },
    ],
  },
];

export const USER_NAV: NavItem[] = [
  { id: 'dashboard', label: 'My Dashboard', to: '/dashboard', icon: 'dashboard' },
  { id: 'datasets', label: 'My Datasets', to: '/storage/datasets', icon: 'dataset' },
  { id: 'shares', label: 'My Shares', to: '/sharing/shares', icon: 'share' },
  { id: 'snapshots', label: 'My Snapshots', to: '/storage/snapshots', icon: 'snap' },
  { id: 'apps', label: 'My Apps', to: '/apps', icon: 'app' },
  { id: 'vms', label: 'My VMs', to: '/vms', icon: 'vm' },
];
