package cmd

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestGuessTypes(t *testing.T) {
	now := time.Now()
	times, description := GuessTypes([]string{"FFS-465: logout on django session timeout", "8:05"})

	assert.Len(t, times, 1)
	assert.Equal(t, 8, times[0].Hour())
	assert.Equal(t, 5, times[0].Minute())
	assert.Equal(t, now.Month(), times[0].Month(), "Month failed")
	assert.Equal(t, now.Year(), times[0].Year(), "Year failed")
	assert.Equal(t, now.Day(), times[0].Day(), "Day failed")

	assert.NotEmptyf(t, description, fmt.Sprintf(`description must not be empty`))
	assert.Equal(t, description, []string{"FFS-465: logout on django session timeout"})

}
