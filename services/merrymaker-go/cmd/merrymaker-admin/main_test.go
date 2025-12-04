package main

import (
	"io"
	"os"
	"testing"

	"github.com/target/mmk-ui-api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrintRulesJobResultsIncludesFailureBanner(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	defer func() {
		os.Stdout = oldStdout
	}()

	os.Stdout = w

	results := &service.RulesProcessingResults{ErrorsEncountered: 2}
	err = printRulesJobResults(&printRulesJobResultsRequest{
		JobID:   "job-123",
		Key:     "",
		Results: results,
	})
	require.NoError(t, err)

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	output, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	outStr := string(output)
	require.Contains(t, outStr, "Status: failed (rule evaluation errors: 2)")
	require.Contains(t, outStr, "results may be incomplete")
}
