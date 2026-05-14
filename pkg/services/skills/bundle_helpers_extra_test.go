package skills

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestBundleHelperFallbacksExtraCoverage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "skill.md", entryPointFromFiles([]SkillBundleFile{
		{Path: "notes.md"},
		{Path: "skill.md", Role: "skill"},
	}))
	require.Equal(t, defaultSkillEntrypoint, entryPointFromFiles([]SkillBundleFile{
		{Path: "docs/README.md"},
		{Path: defaultSkillEntrypoint},
	}))
	require.Equal(t, "first.md", entryPointFromFiles([]SkillBundleFile{{Path: "first.md"}}))
	require.Empty(t, entryPointFromFiles(nil))

	require.Equal(t, []string{"alpha", "beta"}, normalizedBundleStrings([]string{" beta ", "ALPHA", "", "alpha"}))

	require.Equal(t, "custom-dir", resolvedInstallDirectory(nil, skillBundleManifest{
		InstallHints: skillManifestInstallHints{DirectoryName: "custom-dir"},
	}))
	require.Equal(t, "safe-slug", resolvedInstallDirectory(&models.Skill{
		ID:   "skill-id",
		Slug: "safe-slug",
	}, skillBundleManifest{InstallHints: skillManifestInstallHints{DirectoryName: "../unsafe"}}))
	require.Equal(t, defaultSkillDirectory, resolvedInstallDirectory(&models.Skill{
		ID:   "../unsafe",
		Slug: "../unsafe",
	}, skillBundleManifest{}))
}

func TestSortedBundleFilesOrdersByPathThenDigest(t *testing.T) {
	t.Parallel()

	files := []SkillBundleFile{
		{Path: "b.md", Digest: "sha256:b"},
		{Path: "a.md", Digest: "sha256:z"},
		{Path: "a.md", Digest: "sha256:a"},
	}

	got := sortedBundleFiles(files)
	require.Equal(t, []SkillBundleFile{
		{Path: "a.md", Digest: "sha256:a"},
		{Path: "a.md", Digest: "sha256:z"},
		{Path: "b.md", Digest: "sha256:b"},
	}, got)
	require.Equal(t, "b.md", files[0].Path, "sorting should not mutate caller slice")
}
