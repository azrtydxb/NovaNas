import { createFileRoute } from '@tanstack/react-router';
import { Network } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/network')({
  component: NetworkPage,
});

function NetworkPage() {
  return (
    <ShellScreen
      title='Network'
      subtitle='Interfaces, VLANs, bonds, DNS, firewall.'
      icon={<Network size={28} />}
      upcoming={[
        'Interface list with speed, MTU, link state',
        'Bond and VLAN configuration',
        'Static routes and DNS / mDNS',
        'Firewall zones and rules',
      ]}
    />
  );
}
