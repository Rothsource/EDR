package core

import "net"

type NetInfo struct {
	IPAddress  string
	MACAddress string
}

// GetPrimaryInterface walks the machine's network interfaces and returns the
// IP and MAC from the FIRST active, non-loopback interface with an IPv4
// address. Returning both from the same iface.Addrs() loop guarantees they
// actually correspond to the same physical NIC.
func GetPrimaryInterface() NetInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return NetInfo{} // caller treats empty strings as "detection failed"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // skip disabled interfaces and loopback (127.0.0.1)
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			ip4 := ip.To4() // skip IPv6 for now — keeps this matching a simple text column
			if ip4 == nil {
				continue
			}

			return NetInfo{
				IPAddress:  ip4.String(),
				MACAddress: iface.HardwareAddr.String(),
			}
		}
	}

	return NetInfo{}
}
