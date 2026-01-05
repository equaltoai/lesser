package model

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type round12ErrWriter struct{}

func (round12ErrWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRound12ModelScalars_Duration(t *testing.T) {
	var d Duration

	require.NoError(t, d.UnmarshalGQL(5))
	require.Equal(t, Duration(5), d)

	require.NoError(t, d.UnmarshalGQL(int64(7)))
	require.Equal(t, Duration(7), d)

	require.NoError(t, d.UnmarshalGQL(float64(9)))
	require.Equal(t, Duration(9), d)

	require.NoError(t, d.UnmarshalGQL("11"))
	require.Equal(t, Duration(11), d)

	require.NoError(t, d.UnmarshalGQL("5m30s"))
	require.Equal(t, Duration(330), d)

	require.ErrorIs(t, d.UnmarshalGQL(true), ErrInvalidDurationType)
	require.ErrorIs(t, d.UnmarshalGQL("not-a-duration"), ErrInvalidDurationType)

	var buf bytes.Buffer
	Duration(12).MarshalGQL(&buf)
	require.Equal(t, "12", buf.String())

	require.NotPanics(t, func() { Duration(13).MarshalGQL(round12ErrWriter{}) })
	require.Equal(t, "14s", Duration(14).String())
	require.Equal(t, 15, Duration(15).Seconds())
}

func TestRound12ModelScalars_TimeAndCursor(t *testing.T) {
	var t1 Time
	require.NoError(t, t1.UnmarshalGQL("2024-01-02T03:04:05Z"))
	require.Equal(t, "2024-01-02T03:04:05Z", time.Time(t1).UTC().Format(time.RFC3339))

	require.ErrorIs(t, t1.UnmarshalGQL(123), ErrTimeNotString)
	require.Error(t, t1.UnmarshalGQL("not-rfc3339"))

	var timeBuf bytes.Buffer
	Time(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)).MarshalGQL(&timeBuf)
	require.Equal(t, "\"2024-01-02T03:04:05Z\"", timeBuf.String())
	require.NotPanics(t, func() { Time(time.Now()).MarshalGQL(round12ErrWriter{}) })

	var c Cursor
	require.NoError(t, c.UnmarshalGQL("abc"))
	require.Equal(t, Cursor("abc"), c)
	require.ErrorIs(t, c.UnmarshalGQL(1), ErrCursorNotString)

	var cursorBuf bytes.Buffer
	Cursor("xyz").MarshalGQL(&cursorBuf)
	require.Equal(t, "\"xyz\"", cursorBuf.String())
	require.NotPanics(t, func() { Cursor("bad").MarshalGQL(round12ErrWriter{}) })
}

