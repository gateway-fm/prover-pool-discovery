package pool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gateway-fm/service-pool/pkg/logger"
	"github.com/gateway-fm/service-pool/service"
)

// IServicesList is generic interface for services list
type IServicesList interface {
	// Healthy return slice of all healthy services
	Healthy() []service.IService

	// Next returns next healthy service
	// to take a connection
	Next() service.IService

	// Add service to the list
	Add(srv service.IService)

	// IsServiceExists check is given service is
	// already in list (healthy or jail)
	IsServiceExists(srv service.IService) bool

	// HealthChecks pings the healthy services
	// and update the statuses
	HealthChecks()

	// HealthChecksLoop spawn healthchecks for
	// all healthy services periodically
	HealthChecksLoop()

	// TryUpService recursively try to up service
	TryUpService(srv service.IService, try int)

	// RemoveFromHealthy remove service
	// from healthy slice
	RemoveFromHealthy(index int)

	// ToJail add given unhealthy
	// service to jail map
	ToJail(srv service.IService)

	// RemoveFromJail remove given
	// service from jail map
	RemoveFromJail(srv service.IService)

	// Close stop service list hasrvling
	Close()
}

// ServicesList is service list implementation that
// manage healthchecks, jail and try up mechanics
type ServicesList struct {
	current uint64

	serviceName string

	healthy []service.IService

	jail map[string]service.IService

	muMain sync.Mutex
	muJail sync.Mutex

	tryUpTries int

	checkInterval time.Duration
	tryUpInterval time.Duration

	stop chan struct{}
}

// ServicesListOpts is options that needs
// to configure ServicesList instance
type ServicesListOpts struct {
	TryUpTries     int           // number of attempts to try up service from jail (0 for infinity tries)
	TryUpInterval  time.Duration // interval for try up service from jail
	ChecksInterval time.Duration // healthchecks interval
}

// NewServicesList create new ServiceList instance
// with given configuration
func NewServicesList(serviceName string, opts *ServicesListOpts) IServicesList {
	return &ServicesList{
		serviceName:   serviceName,
		jail:          make(map[string]service.IService),
		tryUpTries:    opts.TryUpTries,
		checkInterval: opts.ChecksInterval,
		tryUpInterval: opts.TryUpInterval,
		stop:          make(chan struct{}),
	}
}

// Healthy return slice of all healthy services
func (l *ServicesList) Healthy() []service.IService {
	defer l.muMain.Unlock()
	l.muMain.Lock()

	return l.healthy
}

// Next returns next healthy service
// to take a connection
func (l *ServicesList) Next() service.IService {
	defer l.muMain.Unlock()
	l.muMain.Lock()

	if len(l.healthy) == 0 {
		return nil
	}

	next := l.nextIndex()
	length := len(l.healthy) + next
	for i := next; i < length; i++ {
		idx := i % len(l.healthy)
		if l.healthy[idx].Status() == service.StatusHealthy {
			if i != next {
				atomic.StoreUint64(&l.current, uint64(idx))
			}
			return l.healthy[idx]
		}
	}
	return nil
}

// Add service to the list
func (l *ServicesList) Add(srv service.IService) {
	defer l.muMain.Unlock()
	l.muMain.Lock()

	l.healthy = append(l.healthy, srv)
	logger.Log().Info(fmt.Sprintf("%s service %s with address %s added to list", l.serviceName, srv.ID(), srv.Address()))
}

// IsServiceExists check is given service is
// already in list (healthy or jail)
func (l *ServicesList) IsServiceExists(srv service.IService) bool {
	if l.isServiceInJail(srv) {
		return true
	}

	if l.isServiceInHealthy(srv) {
		return true
	}

	return false
}

// HealthChecks pings the healthy services
// and update the status
func (l *ServicesList) HealthChecks() {
	for i, srv := range l.Healthy() {
		if srv == nil {
			continue
		}

		// TODO need to implement advanced logging level

		//logger.Log().Info(fmt.Sprintf("checking %s service %s...", l.serviceName, srv.ID()))

		if err := srv.HealthCheck(); err != nil {
			logger.Log().Warn(fmt.Errorf("healthcheck error on %s service %s: %w", l.serviceName, srv.ID(), err).Error())

			l.RemoveFromHealthy(i)
			l.ToJail(srv)

			go l.TryUpService(srv, 0)

			logger.Log().Warn(fmt.Sprintf("%s service %s added to jail", l.serviceName, srv.ID()))
			continue
		}

		//logger.Log().Info(fmt.Sprintf("%s service %s on %s is healthy", l.serviceName, srv.ID(), srv.Address()))
	}
}

// HealthChecksLoop spawn healthchecks for
// all healthy periodically
func (l *ServicesList) HealthChecksLoop() {
	logger.Log().Info("start healthchecks loop")

	for {
		select {
		case <-l.stop:
			logger.Log().Warn("stop healthchecks loop")
			return
		default:
			l.HealthChecks()
			sleep(l.checkInterval, l.stop)
		}
	}
}

// TryUpService recursively try to up service
func (l *ServicesList) TryUpService(srv service.IService, try int) {
	if l.tryUpTries != 0 && try >= l.tryUpTries {
		logger.Log().Warn(fmt.Sprintf("maximum %d try to Up %s service %s reached.... service will remove from service list", l.tryUpTries, l.serviceName, srv.ID()))
		l.RemoveFromJail(srv)
		return
	}

	logger.Log().Info(fmt.Sprintf("%d try to up %s service %s on %s", try, l.serviceName, srv.ID(), srv.Address()))

	if err := srv.HealthCheck(); err != nil {
		logger.Log().Warn(fmt.Errorf("service %s healthcheck error: %w", srv.ID(), err).Error())

		sleep(l.tryUpInterval, l.stop)
		l.TryUpService(srv, try+1)
		return
	}

	logger.Log().Info(fmt.Sprintf("service %s is alive!", srv.ID()))
	l.RemoveFromJail(srv)
	l.Add(srv)
}

// RemoveFromHealthy remove service
// from healthy slice
func (l *ServicesList) RemoveFromHealthy(index int) {
	defer l.muMain.Unlock()
	l.muMain.Lock()

	l.healthy = deleteFromSlice(l.healthy, index)
}

// ToJail add given unhealthy
// service to jail map
func (l *ServicesList) ToJail(srv service.IService) {
	defer l.muJail.Unlock()
	l.muJail.Lock()

	l.jail[srv.ID()] = srv
}

// RemoveFromJail remove given
// service from jail map
func (l *ServicesList) RemoveFromJail(srv service.IService) {
	defer l.muJail.Unlock()
	l.muJail.Lock()

	delete(l.jail, srv.ID())
}

// Close stop service list handling
func (l *ServicesList) Close() {
	close(l.stop)
}

// isServiceInJail check if service exist in jail
func (l *ServicesList) isServiceInJail(srv service.IService) bool {
	defer l.muJail.Unlock()
	l.muJail.Lock()

	if _, ok := l.jail[srv.ID()]; ok {
		return true
	}

	return false
}

// isServiceInHealthy check if service exist in
// healthy slice
func (l *ServicesList) isServiceInHealthy(srv service.IService) bool {
	for _, oldService := range l.Healthy() {
		if srv.ID() == oldService.ID() {
			return true
		}
	}
	return false
}

// nextIndex atomically increase the
// counter and return an index
func (l *ServicesList) nextIndex() int {
	return int(atomic.AddUint64(&l.current, uint64(1)) % uint64(len(l.healthy)))
}
