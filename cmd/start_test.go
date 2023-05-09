package cmd

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestGuessTypes(t *testing.T) {
	now := time.Now()
	timeEntry, err := newTimeEntryFromArgs([]string{"FFS-465: logout on django session timeout", "8:05"})
	assert.Nil(t, err)
	assert.Equal(t, 8, timeEntry.Start.Hour())
	assert.Equal(t, 5, timeEntry.Start.Minute())
	assert.Equal(t, now.Month(), timeEntry.Start.Month(), "Month failed")
	assert.Equal(t, now.Year(), timeEntry.Start.Year(), "Year failed")
	assert.Equal(t, now.Day(), timeEntry.Start.Day(), "Day failed")

	assert.NotEmptyf(t, timeEntry.Description, fmt.Sprintf(`description must not be empty`))
	assert.Equal(t, timeEntry.Description, "FFS-465: logout on django session timeout")

}

func TestGuessTypesFromTo(t *testing.T) {
	now := time.Now()
	timeEntry, err := newTimeEntryFromArgs([]string{"10:14-12:03", "Whatever"})
	assert.Nil(t, err)
	assert.Equal(t, 14, timeEntry.Start.Minute())
	assert.Equal(t, now.Month(), timeEntry.Start.Month(), "Month failed")
	assert.Equal(t, now.Year(), timeEntry.Start.Year(), "Year failed")
	assert.Equal(t, now.Day(), timeEntry.Start.Day(), "Day failed")

	duration := time.Duration(timeEntry.Duration) * time.Second
	to := timeEntry.Start.Add(duration)

	assert.Equal(t, 12, to.Hour())
	assert.Equal(t, 3, to.Minute())
	assert.Equal(t, now.Month(), to.Month(), "Month failed")
	assert.Equal(t, now.Year(), to.Year(), "Year failed")
	assert.Equal(t, now.Day(), to.Day(), "Day failed")

	assert.NotEmptyf(t, timeEntry.Description, fmt.Sprintf(`description must not be empty`))
	assert.Equal(t, timeEntry.Description, "Whatever")

}
