// Package stat -- stuff related to ethtool - link control and status
// Check out https://github.com/torvalds/linux/blob/master/include/uapi/linux/ethtool.h - see ethtool_link_settings{} struct
// C code example -- https://stackoverflow.com/questions/41822920/how-to-get-ethtool-settings
package stat

import (
	"fmt"
	"syscall"
	"unsafe"
)

// ethtool describes ethtool communication channel
type ethtool struct {
	fd int
}

// newEthtool opens communication channel for ethtool.
func newEthtool() (*ethtool, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to open socket: %s", err)
	}

	return &ethtool{fd: fd}, nil
}

// close method closes ethtool communication channel.
func (e *ethtool) close() {
	_ = syscall.Close(e.fd) // ignore errors
}

// EthtoolCmd describes deprecated ethtool_cmd{} C struct used for managing link control and status. DEPRECATED struct
type ethtoolCmd struct { /* ethtool.c: struct ethtool_cmd */
	Cmd           uint32 // Command number = %ETHTOOL_GSET or %ETHTOOL_SSET
	Supported     uint32 // Bitmask of %SUPPORTED_* flags for the link modes and features
	Advertising   uint32 // Bitmask of %ADVERTISED_* flags for the link modes and features
	Speed         uint16 // Low bits of the speed, 1Mb units, 0 to INT_MAX or SPEED_UNKNOWN
	Duplex        uint8  // Duplex mode; one of %DUPLEX_*
	Port          uint8  // Physical connector type; one of %PORT_*
	PhyAddress    uint8  // MDIO address of PHY (transceiver) -- origin 'phy_address'
	Transceiver   uint8  // Historically used to distinguish different possible PHY types
	Autoneg       uint8  // Enable/disable autonegotiation and auto-detection
	MdioSupport   uint8  // Bitmask of %ETH_MDIO_SUPPORTS_* flags for the MDIO protocols -- origin 'mdio_support'
	Maxtxpkt      uint32 // Historically used to report TX IRQ coalescing
	Maxrxpkt      uint32 // Historically used to report RX IRQ coalescing
	SpeedHi       uint16 // High bits of the speed, 1Mb units, 0 to INT_MAX or SPEED_UNKNOWN -- origin 'speed_hi'
	EthTpMdix     uint8  // Ethernet twisted-pair MDI(-X) status -- origin 'eth_tp_mdix'
	EthTpMdixCtrl uint8  // Ethernet twisted pair MDI(-X) control -- origin 'eth_tp_mdix_ctrl'
	LpAdvertising uint32 // Bitmask of %ADVERTISED_* flags for the link modes and features -- origin 'lp_advertising'
	Reserved      [2]uint32
}

// EthtoolLinkSettings describes newer ethtool_link_settings{} C struct for managing link control and status. NEWER struct
type ethtoolLinkSettings struct {
	Cmd                 uint32 // Command number = %ETHTOOL_GLINKSETTINGS or %ETHTOOL_SLINKSETTINGS
	Speed               uint32 // Link speed (Mbps)
	Duplex              uint8  // Duplex mode; one of %DUPLEX_*
	Port                uint8  // Physical connector type; one of %PORT_*
	PhyAddress          uint8  // MDIO address of PHY (transceiver) -- origin 'phy_address'
	Autoneg             uint8  // Enable/disable autonegotiation and auto-detection
	MdioSupport         uint8  // Bitmask of %ETH_MDIO_SUPPORTS_* flags for the MDIO protocols supported by the interface -- origin 'mdio_support'
	EthTpMdix           uint8  // Ethernet twisted-pair MDI(-X) status -- origin 'eth_tp_mdix'
	EthTpMdixCtrl       uint8  // Ethernet twisted pair MDI(-X) control -- origin 'eth_tp_mdix_ctrl'
	LinkModeMasksNwords uint8  // Number of 32-bit words for each of the supported, advertising, lp_advertising link mode bitmaps. -- origin 'link_mode_masks_nwords'
	Transceiver         uint8  // Used to distinguish different possible PHY types
	Reserved1           [3]uint8
	Reserved            [7]uint32
	LinkModeMasks       [0]uint32 // -- origin 'link_mode_masks'
}

//
type ifreq struct {
	ifrName [ifNameSize]byte
	ifrData uintptr
}

const (
	ethtoolGset          = 0x00000001 /* get settings -- DEPRECATED */
	ethtoolGlinkSettings = 0x0000004c /* get ethtool_link_settings, should be used instead of ethtool_cmd and ETHTOOL_GSET */
	ifNameSize           = 16         /* maximum size of an interface name */
	siocEthtool          = 0x8946     /* ioctl ethtool request */
	duplexHalf           = 0
	duplexFull           = 1
	duplexUnknown        = 255
)

// getLinkSettings asks network interface settings using ethtool.
func getLinkSettings(ifname string) (int64, int64, error) {
	e, err := newEthtool()
	if err != nil {
		return 0, 0, fmt.Errorf("new ethtool failed: %s", err)
	}
	defer e.close()

	ecmd := ethtoolCmd{Cmd: ethtoolGset}

	var name [ifNameSize]byte
	copy(name[:], ifname)

	ifr := ifreq{
		ifrName: name,
		ifrData: uintptr(unsafe.Pointer(&ecmd)),
	}

	_, _, ep := syscall.Syscall(syscall.SYS_IOCTL, uintptr(e.fd), siocEthtool, uintptr(unsafe.Pointer(&ifr)))
	if ep != 0 {
		return 0, 0, fmt.Errorf("ioctl failed: %s", ep)
	}

	//var speedval uint32 = (uint32(ecmd.Speed_hi) << 16) | (uint32(ecmd.Speed) & 0xffff)

	return int64(ecmd.Speed) * 1000000, int64(ecmd.Duplex), nil
}
