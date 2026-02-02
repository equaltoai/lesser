#!/bin/bash

# Script to help migrate lift tests from MockStorageAdapter to repository pattern

# Find all test files that use MockStorageAdapter
echo "=== Finding test files that need updating ==="
test_files=$(grep -l "MockStorageAdapter" cmd/api/handlers/*_test.go)
echo "Found $(echo "$test_files" | wc -l) files to update"

# Common replacements
for file in $test_files; do
    echo "Processing: $file"
    
    # Replace MockStorageAdapter references
    sed -i.bak 's/var mockStore \*MockStorageAdapter/var mockRepos *MockRepositoryStorage/g' "$file"
    sed -i.bak 's/mockStore = new(MockStorageAdapter)/mockRepos = NewMockRepositoryStorage()/g' "$file"
    sed -i.bak 's/store:  mockStore/repos:  mockRepos/g' "$file"
    sed -i.bak 's/store: mockStore/repos: mockRepos/g' "$file"
    
    # Replace common method calls
    sed -i.bak 's/mockStore\.On("GetActorByNumericID"/mockActorRepo.On("GetActorByNumericID"/g' "$file"
    sed -i.bak 's/mockStore\.On("GetActor"/mockActorRepo.On("GetActor"/g' "$file"
    sed -i.bak 's/mockStore\.On("GetFollowersCount"/mockActorRepo.On("GetFollowersCount"/g' "$file"
    sed -i.bak 's/mockStore\.On("GetFollowingCount"/mockActorRepo.On("GetFollowingCount"/g' "$file"
    sed -i.bak 's/mockStore\.On("GetStatusCount"/mockStatusRepo.On("GetStatusCount"/g' "$file"
    
    # Clean up backup files
    rm "${file}.bak"
done

echo "=== Basic replacements complete ==="
echo "Note: You'll need to manually:"
echo "1. Add mock repository declarations (var mockActorRepo *MockActorRepository, etc)"
echo "2. Initialize the mocks in setupMocks()"
echo "3. Set the mocks on mockRepos using SetXxxRepo methods"
echo "4. Update any other mockStore method calls to use the appropriate repository mock"
