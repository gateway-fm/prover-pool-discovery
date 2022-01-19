package discovery

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"

	"github.com/gateway-fm/service-pool/service"
)

// ConsulDiscovery is a Consul implementation of
// IServiceDiscovery interface
type ConsulDiscovery struct {
	client *consul.Client
}

// NewConsulDiscovery create new Consul-driven
// service Discovery
func NewConsulDiscovery(addr ...string) (IServiceDiscovery, error) {
	if len(addr) != 1 {
		return nil, ErrInvalidArgumentsLength{length: len(addr), driver: DriverConsul}
	}

	config := consul.DefaultConfig()
	if addr[0] != "" {
		config.Address = addr[0]
	}

	c, err := consul.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("connect to consul discovery: %w", err)
	}

	return &ConsulDiscovery{client: c}, nil
}

// Discover and return list of the active
// blockchain addresses for requested networks
func (d *ConsulDiscovery) Discover(service string) ([]service.IService, error) {
	addrs, _, err := d.client.Health().Service(service, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("discover %s service: %w", service, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("discover service via consul: %w", ErrServiceNotFound{service})
	}

	return d.createNodesFromServices(addrs), nil
}

// createNodesFromServices create addresses slice
// from consul addresses
func (d *ConsulDiscovery) createNodesFromServices(consulServices []*consul.ServiceEntry) (services []service.IService) {
	for _, s := range consulServices {
		services = append(services, d.createServiceFromConsul(s))
	}
	return
}

// createServiceFromConsul create BaseService model
// instance from consul service
func (d *ConsulDiscovery) createServiceFromConsul(srv *consul.ServiceEntry) service.IService {
	return service.NewService(fmt.Sprintf("%s:%d", srv.Service.Address, srv.Service.Port), "")
}
