// Stuff related to ethtool - link control and status
// Check out https://github.com/torvalds/linux/blob/master/include/uapi/linux/ethtool.h - see struct ethtool_link_settings
// C code example -- https://stackoverflow.com/questions/41822920/how-to-get-ethtool-settings
package stat

import (
	"fmt"
	"syscall"
	"unsafe"
)

type Ethtool struct {
	fd int
}

// struct ethtool_cmd - link control and status - DEPRECATED struct
type EthtoolCmd struct { /* ethtool.c: struct ethtool_cmd */
	Cmd              uint32 // Command number = %ETHTOOL_GSET or %ETHTOOL_SSET
	Supported        uint32 // Bitmask of %SUPPORTED_* flags for the link modes and features
	Advertising      uint32 // Bitmask of %ADVERTISED_* flags for the link modes and features
	Speed            uint16 // Low bits of the speed, 1Mb units, 0 to INT_MAX or SPEED_UNKNOWN
	Duplex           uint8  // Duplex mode; one of %DUPLEX_*
	Port             uint8  // Physical connector type; one of %PORT_*
	Phy_address      uint8  // MDIO address of PHY (transceiver)
	Transceiver      uint8  // Historically used to distinguish different possible PHY types
	Autoneg          uint8  // Enable/disable autonegotiation and auto-detection
	Mdio_support     uint8  // Bitmask of %ETH_MDIO_SUPPORTS_* flags for the MDIO protocols
	Maxtxpkt         uint32 // Historically used to report TX IRQ coalescing
	Maxrxpkt         uint32 // Historically used to report RX IRQ coalescing
	Speed_hi         uint16 // High bits of the speed, 1Mb units, 0 to INT_MAX or SPEED_UNKNOWN
	Eth_tp_mdix      uint8  // Ethernet twisted-pair MDI(-X) status
	Eth_tp_mdix_ctrl uint8  // Ethernet twisted pair MDI(-X) control
	Lp_advertising   uint32 // Bitmask of %ADVERTISED_* flags for the link modes and features
	Reserved         [2]uint32
}

// struct ethtool_link_settings - link control and status - NEWER struct
type EthtoolLinkSettings struct {
	Cmd                    uint32 // Command number = %ETHTOOL_GLINKSETTINGS or %ETHTOOL_SLINKSETTINGS
	Speed                  uint32 // Link speed (Mbps)
	Duplex                 uint8  // Duplex mode; one of %DUPLEX_*
	Port                   uint8  // Physical connector type; one of %PORT_*
	Phy_address            uint8  // MDIO address of PHY (transceiver)
	Autoneg                uint8  // Enable/disable autonegotiation and auto-detection
	Mdio_support           uint8  // Bitmask of %ETH_MDIO_SUPPORTS_* flags for the MDIO protocols supported by the interface
	Eth_tp_mdix            uint8  // Ethernet twisted-pair MDI(-X) status
	Eth_tp_mdix_ctrl       uint8  // Ethernet twisted pair MDI(-X) control
	Link_mode_masks_nwords uint8  // Number of 32-bit words for each of the supported, advertising, lp_advertising link mode bitmaps.
	Transceiver            uint8  // Used to distinguish different possible PHY types
	Reserved1              [3]uint8
	Reserved               [7]uint32
	Link_mode_masks        [0]uint32
}

type ifreq struct {
	ifr_name [IFNAMSIZ]byte
	ifr_data uintptr
}

const (
	ETHTOOL_GSET          = 0x00000001 /* get settings -- DEPRECATED */
	ETHTOOL_GLINKSETTINGS = 0x0000004c /* get ethtool_link_settings, should be used instead of ethtool_cmd and ETHTOOL_GSET */
	IFNAMSIZ              = 16         /* maximum size of an interface name */
	SIOCETHTOOL           = 0x8946     /* ioctl ethtool request */
	DUPLEX_HALF           = 0
	DUPLEX_FULL           = 1
	DUPLEX_UNKNOWN        = 255
)

func GetLinkSettings(ifname string) (uint32, uint8, error) {
	e, err := NewEthtool()
	if err != nil {
		return 0, 0, fmt.Errorf("new ethtool failed: %s", err)
	}
	defer e.Close()

	ecmd := EthtoolCmd{
		Cmd: ETHTOOL_GSET,
	}

	var name [IFNAMSIZ]byte
	copy(name[:], []byte(ifname))

	ifr := ifreq{
		ifr_name: name,
		ifr_data: uintptr(unsafe.Pointer(&ecmd)),
	}

	_, _, ep := syscall.Syscall(syscall.SYS_IOCTL, uintptr(e.fd),
		SIOCETHTOOL, uintptr(unsafe.Pointer(&ifr)))
	if ep != 0 {
		return 0, 0, fmt.Errorf("ioctl failed: %s", syscall.Errno(ep))
	}

	//var speedval uint32 = (uint32(ecmd.Speed_hi) << 16) | (uint32(ecmd.Speed) & 0xffff)

	return uint32(ecmd.Speed) * 1000000, ecmd.Duplex, nil
}

func NewEthtool() (*Ethtool, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to open socket: %s", err)
	}

	return &Ethtool{
		fd: int(fd),
	}, nil
}

func (e *Ethtool) Close() {
	syscall.Close(e.fd)
}
