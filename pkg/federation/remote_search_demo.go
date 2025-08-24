//go:build demo

package federation

import (
	"context"
	"log"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	types "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/httpclient"
)

// DemoInstanceRegistry is a simple implementation for demonstration
type DemoInstanceRegistry struct {
	instances []*types.Instance
}

func (d *DemoInstanceRegistry) ListHealthyInstances(_ context.Context) ([]*types.Instance, error) {
	return d.instances, nil
}

// DemonstrateDynamicInstanceDiscovery shows how the enhanced remote search works
func DemonstrateDynamicInstanceDiscovery() {
	log.Println("=== Remote Search Dynamic Instance Discovery Demo ===")

	// Create a demo instance registry with some known instances
	registry := &DemoInstanceRegistry{
		instances: []*types.Instance{
			{
				Domain: "mastodon.social",
				Status: types.InstanceStatusActive,
			},
			{
				Domain: "fosstodon.org",
				Status: types.InstanceStatusActive,
			},
			{
				Domain: "mas.to",
				Status: types.InstanceStatusActive,
			},
		},
	}

	// Create remote search service with dynamic discovery
	// Note: In real usage, you would pass your actual storage implementation
	service := &RemoteSearchService{
		instanceRegistry: registry,
		httpClient:       httpclient.NewSecureClient(httpclient.WithTimeout(30 * time.Second)),
		logger:           common.Logger(),
	}

	// Demonstrate dynamic instance discovery
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Testing dynamic instance discovery...")

	// Test the registry-based discovery
	log.Println("Step 1: Getting instances from registry...")
	registryInstances, err := service.getInstancesFromRegistry(ctx)
	if err != nil {
		log.Printf("Registry discovery failed: %v", err)
	} else {
		log.Printf("Found %d instances from registry: %v", len(registryInstances), registryInstances)
	}

	// Test NodeInfo-based discovery
	log.Println("Step 2: Testing NodeInfo discovery...")
	nodeInfoInstances := service.discoverInstancesViaNodeInfo(ctx)
	log.Printf("Found %d instances via NodeInfo: %v", len(nodeInfoInstances), nodeInfoInstances)

	// Test deduplication
	log.Println("Step 3: Testing deduplication...")
	allInstances := append(registryInstances, nodeInfoInstances...)
	allInstances = append(allInstances, "mastodon.social", "MASTODON.SOCIAL") // Add duplicates
	uniqueInstances := service.deduplicateDomains(allInstances)
	log.Printf("Unique instances after deduplication: %v", uniqueInstances)

	// Test health filtering (this will make actual HTTP requests)
	log.Println("Step 4: Testing health filtering...")
	log.Printf("Testing health of first 3 unique instances (making real HTTP requests)...")

	testInstances := uniqueInstances
	if len(testInstances) > 3 {
		testInstances = testInstances[:3]
	}

	healthyInstances := service.filterHealthyInstances(ctx, testInstances)
	log.Printf("Healthy instances: %v", healthyInstances)

	// Demonstrate fallback
	log.Println("Step 5: Testing verified fallback instances...")
	fallbackInstances := service.getVerifiedFallbackInstances(ctx)
	log.Printf("Verified fallback instances: %v", fallbackInstances)

	log.Println("=== Demo Complete ===")
	log.Println("")
	log.Println("Key improvements implemented:")
	log.Println("1. Dynamic instance discovery from federation registry")
	log.Println("2. NodeInfo-based peer discovery")
	log.Println("3. Health checking and filtering")
	log.Println("4. Intelligent fallback to verified instances")
	log.Println("5. No more hardcoded static lists!")
}
