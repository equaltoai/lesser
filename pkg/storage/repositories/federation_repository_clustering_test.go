package repositories

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClusterByConnectionStrength_EmptyNodes verifies empty input returns empty slice
func TestClusterByConnectionStrength_EmptyNodes(t *testing.T) {
	repo := &FederationRepository{}

	// Empty nodes
	result := repo.clusterByConnectionStrength(nil, nil)
	assert.Empty(t, result, "Empty nodes should return empty clusters")

	// Empty slice
	result = repo.clusterByConnectionStrength([]*storage.FederationNode{}, []*storage.FederationEdge{})
	assert.Empty(t, result, "Empty slice should return empty clusters")
}

// TestClusterByConnectionStrength_TwoNodesStrongEdge verifies two nodes with strong connection form a cluster
func TestClusterByConnectionStrength_TwoNodesStrongEdge(t *testing.T) {
	repo := &FederationRepository{}

	nodes := []*storage.FederationNode{
		{Domain: "mastodon.social", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "mastodon.online", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
	}

	edges := []*storage.FederationEdge{
		{
			SourceDomain: "mastodon.social",
			TargetDomain: "mastodon.online",
			Strength:     0.8, // Strong connection (> 0.5 threshold)
		},
	}

	result := repo.clusterByConnectionStrength(nodes, edges)

	require.Len(t, result, 1, "Should create one cluster with both instances")
	assert.Len(t, result[0].Instances, 2, "Cluster should contain both instances")
	assert.Contains(t, result[0].Instances, "mastodon.social")
	assert.Contains(t, result[0].Instances, "mastodon.online")
	assert.Equal(t, 2, result[0].Size, "Cluster size should be 2")
}

// TestClusterByConnectionStrength_ThresholdBehavior verifies threshold filtering
func TestClusterByConnectionStrength_ThresholdBehavior(t *testing.T) {
	repo := &FederationRepository{}

	nodes := []*storage.FederationNode{
		{Domain: "domain-a.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "domain-b.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "domain-c.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
	}

	edges := []*storage.FederationEdge{
		{
			SourceDomain: "domain-a.com",
			TargetDomain: "domain-b.com",
			Strength:     0.7, // Above threshold (0.5)
		},
		{
			SourceDomain: "domain-b.com",
			TargetDomain: "domain-c.com",
			Strength:     0.3, // Below threshold (0.5)
		},
	}

	result := repo.clusterByConnectionStrength(nodes, edges)

	// domain-a and domain-b should be clustered together
	// domain-c will NOT be in a cluster because:
	// 1. It's visited in the first loop (all nodes are visited)
	// 2. It forms a size-1 cluster which is skipped (only size > 1 clusters are added from first loop)
	// 3. The second loop only adds UNVISITED nodes, but domain-c is already visited
	foundClusterWithAB := false

	for _, cluster := range result {
		if len(cluster.Instances) == 2 {
			if contains(cluster.Instances, "domain-a.com") && contains(cluster.Instances, "domain-b.com") {
				foundClusterWithAB = true
				assert.NotContains(t, cluster.Instances, "domain-c.com", "Weak connection should not be included")
			}
		}
	}

	assert.True(t, foundClusterWithAB, "Should cluster domain-a and domain-b with strong connection")

	// Verify domain-c is NOT included in any cluster (this is the actual behavior)
	for _, cluster := range result {
		assert.NotContains(t, cluster.Instances, "domain-c.com",
			"domain-c with weak connection should not appear in any cluster")
	}
}

// TestClusterByConnectionStrength_RecursiveThresholdIncrease verifies threshold increases to prevent sprawl
func TestClusterByConnectionStrength_RecursiveThresholdIncrease(t *testing.T) {
	repo := &FederationRepository{}

	// Create a chain: A -> B -> C -> D with decreasing strength
	nodes := []*storage.FederationNode{
		{Domain: "node-a.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "node-b.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "node-c.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "node-d.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
	}

	edges := []*storage.FederationEdge{
		{SourceDomain: "node-a.com", TargetDomain: "node-b.com", Strength: 0.7},  // Above 0.5
		{SourceDomain: "node-b.com", TargetDomain: "node-c.com", Strength: 0.55}, // Above 0.5, but below 0.6 (0.5+0.1 for recursive)
		{SourceDomain: "node-c.com", TargetDomain: "node-d.com", Strength: 0.52}, // Will fail recursive threshold test
	}

	result := repo.clusterByConnectionStrength(nodes, edges)

	// Due to recursive threshold increase (+0.1), the chain should not sprawl indefinitely
	// The exact behavior depends on traversal order, but we verify at least some clustering happens
	require.NotEmpty(t, result, "Should produce at least one cluster")

	// Verify no single cluster contains all 4 nodes (sprawl prevention)
	for _, cluster := range result {
		if cluster.Size > 1 {
			// Any multi-node cluster should have reasonable cohesion
			assert.LessOrEqual(t, cluster.Size, 3, "Cluster should not sprawl to include all nodes due to threshold increase")
		}
	}
}

// TestClusterByConnectionStrength_SingletonClusters verifies isolated nodes behavior
// NOTE: The actual implementation only creates singleton clusters for nodes that are
// NOT in the input nodes slice but are discovered through edges and are unvisited.
// For nodes in the input that have no strong connections, they are visited but not
// added to any cluster because they form size-1 clusters which are skipped.
func TestClusterByConnectionStrength_SingletonClusters(t *testing.T) {
	repo := &FederationRepository{}

	nodes := []*storage.FederationNode{
		{Domain: "lonely-a.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "lonely-b.com", Software: "pleroma", Health: ""}, // Empty health treated as healthy
	}

	// No edges - these are isolated nodes
	edges := []*storage.FederationEdge{}

	result := repo.clusterByConnectionStrength(nodes, edges)

	// With the current implementation:
	// - All nodes are visited in the first loop
	// - Size-1 clusters are not added from the first loop
	// - The second loop only adds UNVISITED healthy nodes (none exist)
	// So the result should be empty
	assert.Empty(t, result, "Isolated nodes in input are visited but not added as clusters")
}

// =============================================================================
// Tests for findCenterNode
// =============================================================================

// TestFindCenterNode_SingleInstance verifies single instance returns itself
func TestFindCenterNode_SingleInstance(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"only-one.com"}
	connectionMap := map[string]map[string]float64{
		"only-one.com": {},
	}
	nodes := []*storage.FederationNode{
		{Domain: "only-one.com", Health: string(monitoring.HealthStatusHealthy)},
	}

	result := repo.findCenterNode(instances, connectionMap, nodes)
	assert.Equal(t, "only-one.com", result, "Single instance should return itself as center")
}

// TestFindCenterNode_EmptyInstances verifies empty input returns empty string
func TestFindCenterNode_EmptyInstances(t *testing.T) {
	repo := &FederationRepository{}

	result := repo.findCenterNode(nil, nil, nil)
	assert.Empty(t, result, "Empty instances should return empty string")

	result = repo.findCenterNode([]string{}, nil, nil)
	assert.Empty(t, result, "Empty slice should return empty string")
}

// TestFindCenterNode_HealthyVsUnhealthy verifies healthy node is preferred as center
func TestFindCenterNode_HealthyVsUnhealthy(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"healthy.com", "unhealthy.com"}
	connectionMap := map[string]map[string]float64{
		"healthy.com":   {"unhealthy.com": 0.8},
		"unhealthy.com": {"healthy.com": 0.8},
	}
	nodes := []*storage.FederationNode{
		{Domain: "healthy.com", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "unhealthy.com", Health: "unhealthy"},
	}

	result := repo.findCenterNode(instances, connectionMap, nodes)

	// With equal connection strength (0.8 each), health weight tips the scale
	// healthy: 0.8 * 1.0 = 0.8
	// unhealthy: 0.8 * 0.4 = 0.32
	assert.Equal(t, "healthy.com", result, "Healthy node should be preferred as center")
}

// TestFindCenterNode_HealthWeights verifies different health statuses have appropriate weights
func TestFindCenterNode_HealthWeights(t *testing.T) {
	tests := []struct {
		name           string
		healthStatus   string
		expectedWeight float64
	}{
		{"healthy", string(monitoring.HealthStatusHealthy), 1.0},
		{"degraded", "degraded", 0.7},
		{"unhealthy", "unhealthy", 0.4},
		{"critical", string(monitoring.HealthStatusCritical), 0.1},
		{"unknown empty", "", 0.8}, // Default for unknown/empty
		{"random value", "some-other-status", 0.8},
	}

	repo := &FederationRepository{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scenario where health weight determines the outcome
			instances := []string{"test-node.com", "reference.com"}
			connectionMap := map[string]map[string]float64{
				"test-node.com": {"reference.com": 1.0},
				"reference.com": {"test-node.com": 1.0},
			}
			nodes := []*storage.FederationNode{
				{Domain: "test-node.com", Health: tt.healthStatus},
				{Domain: "reference.com", Health: string(monitoring.HealthStatusCritical)}, // Very low weight (0.1)
			}

			result := repo.findCenterNode(instances, connectionMap, nodes)

			// test-node has score: 1.0 * tt.expectedWeight
			// reference has score: 1.0 * 0.1 (critical)
			// If tt.expectedWeight > 0.1, test-node wins
			if tt.expectedWeight > 0.1 {
				assert.Equal(t, "test-node.com", result, "Node with higher health weight should be center")
			} else {
				// Both have same weight (critical), first one wins
				assert.NotEmpty(t, result, "Should return some center node")
			}
		})
	}
}

// TestFindCenterNode_MostConnectedNode verifies node with most connections is preferred
func TestFindCenterNode_MostConnectedNode(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"hub.com", "spoke1.com", "spoke2.com"}
	connectionMap := map[string]map[string]float64{
		// hub connects to both spokes
		"hub.com": {"spoke1.com": 0.8, "spoke2.com": 0.8},
		// spokes only connect to hub
		"spoke1.com": {"hub.com": 0.8},
		"spoke2.com": {"hub.com": 0.8},
	}
	nodes := []*storage.FederationNode{
		{Domain: "hub.com", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "spoke1.com", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "spoke2.com", Health: string(monitoring.HealthStatusHealthy)},
	}

	result := repo.findCenterNode(instances, connectionMap, nodes)

	// hub.com has total strength = 0.8 + 0.8 = 1.6
	// spoke1.com has total strength = 0.8
	// spoke2.com has total strength = 0.8
	assert.Equal(t, "hub.com", result, "Node with most connections should be center")
}

// =============================================================================
// Tests for calculateCohesion
// =============================================================================

// TestCalculateCohesion_SingleInstance returns 1.0
func TestCalculateCohesion_SingleInstance(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"solo.com"}
	connectionMap := map[string]map[string]float64{
		"solo.com": {},
	}

	result := repo.calculateCohesion(instances, connectionMap)
	assert.Equal(t, 1.0, result, "Single instance should have cohesion 1.0")
}

// TestCalculateCohesion_EmptyInstances returns 1.0
func TestCalculateCohesion_EmptyInstances(t *testing.T) {
	repo := &FederationRepository{}

	result := repo.calculateCohesion(nil, nil)
	assert.Equal(t, 1.0, result, "Empty instances should return 1.0")

	result = repo.calculateCohesion([]string{}, nil)
	assert.Equal(t, 1.0, result, "Empty slice should return 1.0")
}

// TestCalculateCohesion_TwoInstances_FullStrength returns edge strength
func TestCalculateCohesion_TwoInstances_FullStrength(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"a.com", "b.com"}
	connectionMap := map[string]map[string]float64{
		"a.com": {"b.com": 0.8},
		"b.com": {"a.com": 0.8},
	}

	result := repo.calculateCohesion(instances, connectionMap)

	// 2 instances = 1 possible connection
	// Actual connection strength = 0.8
	// Cohesion = 0.8 / 1 = 0.8
	assert.Equal(t, 0.8, result, "Cohesion should equal edge strength for two nodes")
}

// TestCalculateCohesion_ThreeInstances_PartialConnections returns average
func TestCalculateCohesion_ThreeInstances_PartialConnections(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"a.com", "b.com", "c.com"}
	connectionMap := map[string]map[string]float64{
		"a.com": {"b.com": 0.8, "c.com": 0.6},
		"b.com": {"a.com": 0.8}, // Only connected to a.com, not c.com
		"c.com": {"a.com": 0.6},
	}

	result := repo.calculateCohesion(instances, connectionMap)

	// 3 instances = 3 possible connections (a-b, a-c, b-c)
	// Actual: a-b = 0.8, a-c = 0.6, b-c = 0 (no connection)
	// Total strength = 0.8 + 0.6 = 1.4
	// Cohesion = 1.4 / 3 ≈ 0.4667
	expectedCohesion := 1.4 / 3.0
	assert.InDelta(t, expectedCohesion, result, 0.001, "Cohesion should be average strength / possible connections")
}

// TestCalculateCohesion_FullyConnected returns high cohesion
func TestCalculateCohesion_FullyConnected(t *testing.T) {
	repo := &FederationRepository{}

	instances := []string{"a.com", "b.com", "c.com"}
	connectionMap := map[string]map[string]float64{
		"a.com": {"b.com": 0.9, "c.com": 0.9},
		"b.com": {"a.com": 0.9, "c.com": 0.9},
		"c.com": {"a.com": 0.9, "b.com": 0.9},
	}

	result := repo.calculateCohesion(instances, connectionMap)

	// 3 instances = 3 possible connections
	// All edges have strength 0.9
	// Total strength = 0.9 + 0.9 + 0.9 = 2.7
	// Cohesion = 2.7 / 3 = 0.9
	assert.Equal(t, 0.9, result, "Fully connected cluster should have cohesion equal to edge strength")
}

// =============================================================================
// Tests for generateClusterDescription
// =============================================================================

// TestGenerateClusterDescription_SingleInstance format
func TestGenerateClusterDescription_SingleInstance(t *testing.T) {
	repo := &FederationRepository{}

	cluster := &storage.InstanceCluster{
		Size:      1,
		Instances: []string{"solo.com"},
	}
	nodes := []*storage.FederationNode{
		{Domain: "solo.com", Software: "mastodon"},
	}

	result := repo.generateClusterDescription(cluster, nodes)

	assert.Contains(t, result, "Single instance cluster", "Should indicate single instance")
	assert.Contains(t, result, "solo.com", "Should include domain name")
}

// TestGenerateClusterDescription_DominantSoftware selects most common software
func TestGenerateClusterDescription_DominantSoftware(t *testing.T) {
	repo := &FederationRepository{}

	cluster := &storage.InstanceCluster{
		Size:       4,
		Instances:  []string{"a.com", "b.com", "c.com", "d.com"},
		CenterNode: "a.com",
		Cohesion:   0.8,
	}
	nodes := []*storage.FederationNode{
		{Domain: "a.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "b.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "c.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "d.com", Software: "pleroma", Health: string(monitoring.HealthStatusHealthy)},
	}

	result := repo.generateClusterDescription(cluster, nodes)

	// mastodon appears 3 times, pleroma 1 time
	assert.Contains(t, result, "mastodon", "Should mention dominant software")
	assert.Contains(t, result, "4", "Should include cluster size")
}

// TestGenerateClusterDescription_CohesionBuckets verifies cohesion level descriptions
func TestGenerateClusterDescription_CohesionBuckets(t *testing.T) {
	tests := []struct {
		name     string
		cohesion float64
		expected string
	}{
		{"high cohesion > 0.7", 0.85, "tightly"},
		{"medium cohesion > 0.4", 0.55, "moderately"},
		{"low cohesion <= 0.4", 0.3, "loosely"},
		{"boundary at 0.7", 0.71, "tightly"},
		{"boundary at 0.4", 0.41, "moderately"},
		{"exactly 0.7", 0.7, "moderately"}, // 0.7 is NOT > 0.7
		{"exactly 0.4", 0.4, "loosely"},    // 0.4 is NOT > 0.4
	}

	repo := &FederationRepository{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &storage.InstanceCluster{
				Size:       2,
				Instances:  []string{"a.com", "b.com"},
				CenterNode: "a.com",
				Cohesion:   tt.cohesion,
			}
			nodes := []*storage.FederationNode{
				{Domain: "a.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
				{Domain: "b.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
			}

			result := repo.generateClusterDescription(cluster, nodes)

			assert.Contains(t, result, tt.expected, "Cohesion %.2f should be '%s' connected", tt.cohesion, tt.expected)
		})
	}
}

// TestGenerateClusterDescription_HealthyCount counts healthy nodes
func TestGenerateClusterDescription_HealthyCount(t *testing.T) {
	repo := &FederationRepository{}

	cluster := &storage.InstanceCluster{
		Size:       3,
		Instances:  []string{"healthy1.com", "healthy2.com", "unhealthy.com"},
		CenterNode: "healthy1.com",
		Cohesion:   0.6,
	}
	nodes := []*storage.FederationNode{
		{Domain: "healthy1.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "healthy2.com", Software: "mastodon", Health: ""},          // Empty treated as healthy
		{Domain: "unhealthy.com", Software: "mastodon", Health: "degraded"}, // Not healthy
	}

	result := repo.generateClusterDescription(cluster, nodes)

	// 2 healthy nodes (healthy1 + empty health healthy2)
	assert.Contains(t, result, "2 healthy", "Should count 2 healthy nodes")
}

// TestGenerateClusterDescription_CenterNodeIncluded verifies center node is mentioned
func TestGenerateClusterDescription_CenterNodeIncluded(t *testing.T) {
	repo := &FederationRepository{}

	cluster := &storage.InstanceCluster{
		Size:       2,
		Instances:  []string{"main.com", "secondary.com"},
		CenterNode: "main.com",
		Cohesion:   0.8,
	}
	nodes := []*storage.FederationNode{
		{Domain: "main.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
		{Domain: "secondary.com", Software: "mastodon", Health: string(monitoring.HealthStatusHealthy)},
	}

	result := repo.generateClusterDescription(cluster, nodes)

	assert.Contains(t, result, "center: main.com", "Should include center node")
}

// =============================================================================
// Tests for addConnectedNodes (indirectly tested through clusterByConnectionStrength)
// =============================================================================

// TestAddConnectedNodes_ThresholdFiltering verifies only strong edges are followed
func TestAddConnectedNodes_ThresholdFiltering(t *testing.T) {
	repo := &FederationRepository{}

	// Setup connection map with mixed strengths
	connectionMap := map[string]map[string]float64{
		"start.com": {
			"strong.com": 0.7,  // Above threshold
			"weak.com":   0.3,  // Below threshold
			"border.com": 0.51, // Just above threshold
		},
		"strong.com": {"start.com": 0.7},
		"weak.com":   {"start.com": 0.3},
		"border.com": {"start.com": 0.51},
	}

	visited := map[string]bool{"start.com": true}
	cluster := &storage.InstanceCluster{
		Instances: []string{"start.com"},
	}

	// Threshold of 0.5 means only edges > 0.5 are followed
	repo.addConnectedNodes("start.com", connectionMap, visited, cluster, 0.5)

	assert.Contains(t, cluster.Instances, "strong.com", "Strong connection should be added")
	assert.Contains(t, cluster.Instances, "border.com", "Border connection (0.51 > 0.5) should be added")
	assert.NotContains(t, cluster.Instances, "weak.com", "Weak connection should not be added")
}

// =============================================================================
// Helper function
// =============================================================================

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
