package dhcp6

import (
	"hash/fnv"
	"encoding/binary"
	"net"
)

type PacketBuilder struct {
	ServerDuid        []byte
	PreferredLifetime uint32
	ValidLifetime     uint32
}

func MakePacketBuilder(serverDuid []byte, preferredLifetime, validLifetime uint32) *PacketBuilder {
	return &PacketBuilder{ServerDuid: serverDuid, PreferredLifetime: preferredLifetime, ValidLifetime: validLifetime}
}

func (b *PacketBuilder) BuildResponse(in *Packet, configuration BootConfiguration, addresses AddressPool) (*Packet, error) {
	switch in.Type {
	case MsgSolicit:
		bootFileUrl, err := configuration.GetBootUrl(b.ExtractLLAddressOrId(in.Options.ClientId()), in.Options.ClientArchType())
		if err != nil {
			return nil, err
		}
		associations, err := addresses.ReserveAddresses(in.Options.ClientId(), in.Options.IaNaIds())
		if err != nil {
			return b.MakeMsgAdvertiseWithNoAddrsAvailable(in.TransactionID, in.Options.ClientId(), err), err
		}
		return b.MakeMsgAdvertise(in.TransactionID, in.Options.ClientId(),
			in.Options.ClientArchType(), associations, bootFileUrl, configuration.GetPreference(), configuration.GetRecursiveDns()), nil
	case MsgRequest:
		bootFileUrl, err := configuration.GetBootUrl(b.ExtractLLAddressOrId(in.Options.ClientId()), in.Options.ClientArchType())
		if err != nil {
			return nil, err
		}
		associations, err := addresses.ReserveAddresses(in.Options.ClientId(), in.Options.IaNaIds())
		return b.MakeMsgReply(in.TransactionID, in.Options.ClientId(),
				in.Options.ClientArchType(), associations, iasWithoutAddesses(associations, in.Options.IaNaIds()), bootFileUrl,
				configuration.GetRecursiveDns(), err), err
	case MsgInformationRequest:
		bootFileUrl, err := configuration.GetBootUrl(b.ExtractLLAddressOrId(in.Options.ClientId()), in.Options.ClientArchType())
		if err != nil {
			return nil, err
		}
		return b.MakeMsgInformationRequestReply(in.TransactionID, in.Options.ClientId(),
			in.Options.ClientArchType(), bootFileUrl, configuration.GetRecursiveDns()), nil
	case MsgRelease:
		addresses.ReleaseAddresses(in.Options.ClientId(), in.Options.IaNaIds())
		return b.MakeMsgReleaseReply(in.TransactionID, in.Options.ClientId()), nil
	default:
		return nil, nil
	}
}

func (b *PacketBuilder) MakeMsgAdvertise(transactionId [3]byte, clientId []byte, clientArchType uint16,
	associations []*IdentityAssociation, bootFileUrl, preference []byte, dnsServers []net.IP) *Packet {
	ret_options := make(Options)
	ret_options.AddOption(MakeOption(OptClientId, clientId))
	for _, association := range(associations) {
		ret_options.AddOption(MakeIaNaOption(association.InterfaceId, b.calculateT1(), b.calculateT2(),
			MakeIaAddrOption(association.IpAddress, b.PreferredLifetime, b.ValidLifetime)))
	}
	ret_options.AddOption(MakeOption(OptServerId, b.ServerDuid))
	if 0x10 ==  clientArchType { // HTTPClient
		ret_options.AddOption(MakeOption(OptVendorClass, []byte {0, 0, 0, 0, 0, 10, 72, 84, 84, 80, 67, 108, 105, 101, 110, 116})) // HTTPClient
	}
	ret_options.AddOption(MakeOption(OptBootfileUrl, bootFileUrl))
	if preference != nil {ret_options.AddOption(MakeOption(OptPreference, preference))}
	ret_options.AddOption(MakeDNSServersOption(dnsServers))

	return &Packet{Type: MsgAdvertise, TransactionID: transactionId, Options: ret_options}
}

func (b *PacketBuilder) MakeMsgReply(transactionId [3]byte, clientId []byte, clientArchType uint16,
	associations []*IdentityAssociation, iasWithoutAddresses [][]byte, bootFileUrl []byte, dnsServers []net.IP, err error) *Packet {
	ret_options := make(Options)
	ret_options.AddOption(MakeOption(OptClientId, clientId))
	for _, association := range(associations) {
		ret_options.AddOption(MakeIaNaOption(association.InterfaceId, b.calculateT1(), b.calculateT2(),
			MakeIaAddrOption(association.IpAddress, b.PreferredLifetime, b.ValidLifetime)))
	}
	for _, ia := range(iasWithoutAddresses) {
		ret_options.AddOption(MakeIaNaOption(ia, b.calculateT1(), b.calculateT2(),
			MakeStatusOption(2, err.Error())))
	}
	ret_options.AddOption(MakeOption(OptServerId, b.ServerDuid))
	if 0x10 ==  clientArchType { // HTTPClient
		ret_options.AddOption(MakeOption(OptVendorClass, []byte {0, 0, 0, 0, 0, 10, 72, 84, 84, 80, 67, 108, 105, 101, 110, 116})) // HTTPClient
	}
	ret_options.AddOption(MakeOption(OptBootfileUrl, bootFileUrl))
	ret_options.AddOption(MakeDNSServersOption(dnsServers))

	return &Packet{Type: MsgReply, TransactionID: transactionId, Options: ret_options}
}

func (b *PacketBuilder) MakeMsgInformationRequestReply(transactionId [3]byte, clientId []byte, clientArchType uint16,
	bootFileUrl []byte, dnsServers []net.IP) *Packet {
	ret_options := make(Options)
	ret_options.AddOption(MakeOption(OptClientId, clientId))
	ret_options.AddOption(MakeOption(OptServerId, b.ServerDuid))
	if 0x10 ==  clientArchType { // HTTPClient
		ret_options.AddOption(MakeOption(OptVendorClass, []byte {0, 0, 0, 0, 0, 10, 72, 84, 84, 80, 67, 108, 105, 101, 110, 116})) // HTTPClient
	}
	ret_options.AddOption(MakeOption(OptBootfileUrl, bootFileUrl))
	ret_options.AddOption(MakeDNSServersOption(dnsServers))

	return &Packet{Type: MsgReply, TransactionID: transactionId, Options: ret_options}
}

func (b *PacketBuilder) MakeMsgReleaseReply(transactionId [3]byte, clientId []byte) *Packet {
	ret_options := make(Options)

	ret_options.AddOption(MakeOption(OptClientId, clientId))
	ret_options.AddOption(MakeOption(OptServerId, b.ServerDuid))
	v := make([]byte, 19, 19)
	copy(v[2:], []byte("Release received."))
	ret_options.AddOption(MakeOption(OptStatusCode, v))

	return &Packet{Type: MsgReply, TransactionID: transactionId, Options: ret_options}
}

func (b *PacketBuilder) MakeMsgAdvertiseWithNoAddrsAvailable(transactionId [3]byte, clientId []byte, err error) *Packet {
	ret_options := make(Options)
	ret_options.AddOption(MakeOption(OptClientId, clientId))
	ret_options.AddOption(MakeOption(OptServerId, b.ServerDuid))
	ret_options.AddOption(MakeStatusOption(2, err.Error())) // NoAddrAvailable
	return &Packet{Type: MsgAdvertise, TransactionID: transactionId, Options: ret_options}
}

func (b *PacketBuilder) calculateT1() uint32 {
	return b.PreferredLifetime / 2
}

func (b *PacketBuilder) calculateT2() uint32 {
	return (b.PreferredLifetime * 4)/5
}

func (b *PacketBuilder) ExtractLLAddressOrId(optClientId []byte) []byte {
	idType := binary.BigEndian.Uint16(optClientId[0:2])
	switch idType {
	case 1:
		return optClientId[8:]
	case 3:
		return optClientId[4:]
	default:
		return optClientId[2:]
	}
}

func iasWithoutAddesses(availableAssociations []*IdentityAssociation, allIas [][]byte) [][]byte {
	ret := make([][]byte, 0)
	iasWithAddresses := make(map[uint64]bool)

	for _, association := range(availableAssociations) {
		iasWithAddresses[calculateIaIdHash(association.InterfaceId)] = true
	}

	for _, ia := range(allIas) {
		_, exists := iasWithAddresses[calculateIaIdHash(ia)]; if !exists {
			ret = append(ret, ia)
		}
	}
	return ret
}

func calculateIaIdHash(interfaceId []byte) uint64 {
	h := fnv.New64a()
	h.Write(interfaceId)
	return h.Sum64()
}
