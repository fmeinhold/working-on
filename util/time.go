package util

import (
	"fmt"
	"regexp"
	"time"
)

var (
	isTime = regexp.MustCompile("^\\d{1,2}:\\d{2}$")
)

func TimeInUTC(time *time.Time) *time.Time {
	if time != nil {
		utc := time.UTC()
		return &utc
	}
	return nil
}

func ParseTimeUTC(str string, dateLayout string, dateTimeLayout string, loc *time.Location) (dt time.Time) {
	dt, _ = ParseTimeUTCE(str, dateLayout, dateTimeLayout, loc)
	return dt
}

func ParseTimeUTCE(str string, dateLayout string, dateTimeLayout string, loc *time.Location) (dt time.Time, err error) {
	if isTime.MatchString(str) {
		dt = time.Now()
		str = fmt.Sprintf("%s %s", dt.Format(dateLayout), str)
		dt, err = time.ParseInLocation(dateTimeLayout, str, loc)
		if err != nil {
			return time.Time{}, err
		}
		return dt.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("%s is not a time", str)
}

func ParseDateTimeUTC(str string, dateTimeLayout string, loc *time.Location) (dt time.Time) {
	dt, _ = ParseDateTimeUTCE(str, dateTimeLayout, loc)
	return dt
}

// ParseDateTime converts a string into a datetime in UTC
func ParseDateTimeUTCE(str string, dateTimeLayout string, loc *time.Location) (dt time.Time, err error) {
	dt, err = time.ParseInLocation(dateTimeLayout, str, loc)
	if err != nil {
		return time.Time{}, err
	}

	dt = dt.UTC()

	return dt, nil
}

func ParseDate(str string, dateLayout string, loc *time.Location) (dt time.Time) {
	dt, _ = ParseDateE(str, dateLayout, loc)
	return dt
}
func ParseDateE(str string, dateLayout string, loc *time.Location) (dt time.Time, err error) {
	dt, err = time.ParseInLocation(dateLayout, str, loc)
	if err != nil {
		return time.Time{}, err
	}

	dt = dt.UTC()

	return dt, nil
}
