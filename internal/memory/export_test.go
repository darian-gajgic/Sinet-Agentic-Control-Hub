package memory

import "github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"

// export_test.go — test-only access to governance internals, so the black-box
// suite can name the frozen content an Ensure is recorded against without that
// content becoming part of the package's API.

// RW12SoftwareTaxonomyForTest returns the frozen RW-12-ratified software
// question set — what EnsureRW12TaxonomyGovernance writes, which since
// P3-GF3-BE1 is no longer what the runtime seed says.
func RW12SoftwareTaxonomyForTest() *intake.Taxonomy {
	return rw12TaxonomySnapshot()[intake.FamilySoftware]
}

// RW12SoftwareContentHashForTest returns the content hash of that frozen set —
// the value a governed write of RW-12's content records.
func RW12SoftwareContentHashForTest() string {
	content, err := taxonomyContent(intake.FamilySoftware)
	if err != nil {
		panic(err)
	}
	return contentHash(content)
}

// GF3SoftwareTaxonomyForTest returns the frozen P3-GF3-BE1 software question set
// — what EnsureGF3TaxonomyGovernance writes, which since P3-GF7 is no longer
// what the runtime seed says.
func GF3SoftwareTaxonomyForTest() *intake.Taxonomy {
	return gf3TaxonomySnapshot()[intake.FamilySoftware]
}
