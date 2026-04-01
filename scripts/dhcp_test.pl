#!/usr/bin/env perl

use CPAN::AutoINC; # Auto-install dependencies
use Modern::Perl;
use IO::Socket::INET;

$| = 1; # Enable autoflush
use Net::DHCP::Packet;
use Net::DHCP::Constants;

# 1. Create DHCP Discover Packet
my $discover = Net::DHCP::Packet->new(
    DHO_DHCP_MESSAGE_TYPE() => DHCPDISCOVER,
    xid => int(rand(0xFFFFFFFF)),
    chaddr => 'aabbccddeeff', # Using a dummy MAC address
);

# 2. Create and configure UDP socket
my $sock = IO::Socket::INET->new(
    LocalPort => 68,
    PeerAddr  => '255.255.255.255',
    PeerPort  => 67,
    Proto     => 'udp',
    Broadcast => 1,
) or die "Error creating socket: $!\n";

# 3. Send the packet
print "Broadcasting DHCP Discover...\n";
$sock->send($discover->serialize) or die "Error sending packet: $!\n";

# 4. Listen for a response with a 5-second timeout
my $response_raw;
eval {
    local $SIG{ALRM} = sub { die "timeout" };
    alarm 5;
    $sock->recv($response_raw, 1500); # 1500 is a typical MTU
    alarm 0;
};

# 5. Process the response
if ($@ && $@ =~ /timeout/) {
    print "Timeout: No DHCP offers were received.\n";
} elsif ($response_raw) {
    my $offer = Net::DHCP::Packet->new($response_raw);
    my $server_ip = $offer->get_option(DHO_DHCP_SERVER_IDENTIFIER);
    if ($server_ip) {
        print "Received DHCP Offer from server: $server_ip\n";
    } else {
        print "Received a response, but it wasn't a valid DHCP Offer.\n";
    }
} else {
    print "No response received.\n";
}

close($sock);

