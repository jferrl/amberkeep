package migrate

import (
	"context"
	"database/sql"
)

// What an iPhone store's own rows look like, sampled from the store rather than
// written down here.
//
// WhatsApp fills several columns with values that mean something only to itself:
// flags, a status, a spotlight code. A message written with the wrong ones is a
// message the phone may not show, or may show as still sending, years after it was
// sent. The values are not documented anywhere, so the only honest source is the
// store being written into — it was written by the version of WhatsApp that will
// read it back.
//
// Where a store holds no example to copy, the fallbacks are what a real 2026 store
// held, which is where the prototype got them. They are a last resort, not a default.
type conventions struct {
	outgoingFlags  int64
	outgoingStatus int64
	incomingFlags  int64
	incomingStatus int64
	spotlight      int64

	sessionFlags     int64
	spotlightDirect  int64
	spotlightGrouped int64

	// sampled says how many of these came from the store rather than from the
	// fallbacks, so a report can admit when it is guessing.
	sampled int
}

// The values a real store held, used only where the store offers no example.
const (
	fallbackOutgoingFlags  = 16777280
	fallbackOutgoingStatus = 6
	fallbackIncomingFlags  = 16777216
	fallbackIncomingStatus = 13
	fallbackSpotlight      = -32768
	fallbackSessionFlags   = 272
	fallbackSpotlightOne   = -5
	fallbackSpotlightGroup = 1
)

// textMessage is the type code of a plain text message, which is what everything
// carried across becomes.
const textMessage = 0

// sample reads the store's own conventions, falling back where it has no example.
func sample(ctx context.Context, db *sql.DB) conventions {
	c := conventions{
		outgoingFlags: fallbackOutgoingFlags, outgoingStatus: fallbackOutgoingStatus,
		incomingFlags: fallbackIncomingFlags, incomingStatus: fallbackIncomingStatus,
		spotlight: fallbackSpotlight, sessionFlags: fallbackSessionFlags,
		spotlightDirect: fallbackSpotlightOne, spotlightGrouped: fallbackSpotlightGroup,
	}

	// The most common value among this store's own plain text messages, in each
	// direction. Most common rather than any one: a store holds the occasional odd
	// row, and copying an odd one would spread it.
	c.take(ctx, db, &c.outgoingFlags, "ZFLAGS", "ZISFROMME = 1")
	c.take(ctx, db, &c.outgoingStatus, "ZMESSAGESTATUS", "ZISFROMME = 1")
	c.take(ctx, db, &c.incomingFlags, "ZFLAGS", "ZISFROMME = 0")
	c.take(ctx, db, &c.incomingStatus, "ZMESSAGESTATUS", "ZISFROMME = 0")
	c.take(ctx, db, &c.spotlight, "ZSPOTLIGHTSTATUS", "1 = 1")

	c.takeSession(ctx, db, &c.sessionFlags, "ZFLAGS", "1 = 1")
	c.takeSession(ctx, db, &c.spotlightDirect, "ZSPOTLIGHTSTATUS", "ZSESSIONTYPE = 0")
	c.takeSession(ctx, db, &c.spotlightGrouped, "ZSPOTLIGHTSTATUS", "ZSESSIONTYPE = 1")
	return c
}

// take copies the commonest value of one message column.
func (c *conventions) take(ctx context.Context, db *sql.DB, into *int64, column, where string) {
	c.commonest(ctx, db, into,
		"SELECT "+column+" FROM ZWAMESSAGE WHERE "+column+" IS NOT NULL AND ZMESSAGETYPE = 0 AND "+where+
			" GROUP BY 1 ORDER BY count(*) DESC LIMIT 1")
}

// takeSession copies the commonest value of one conversation column.
func (c *conventions) takeSession(ctx context.Context, db *sql.DB, into *int64, column, where string) {
	c.commonest(ctx, db, into,
		"SELECT "+column+" FROM ZWACHATSESSION WHERE "+column+" IS NOT NULL AND "+where+
			" GROUP BY 1 ORDER BY count(*) DESC LIMIT 1")
}

// commonest runs one sampling query, leaving the fallback in place when the store has
// nothing to offer.
//
// The column names are this package's own literals, not anything a store or a user
// supplied, which is why they are concatenated rather than bound: a column name
// cannot be a bound parameter in SQL.
func (c *conventions) commonest(ctx context.Context, db *sql.DB, into *int64, query string) {
	var found sql.NullInt64
	if err := db.QueryRowContext(ctx, query).Scan(&found); err != nil || !found.Valid { // #nosec G202
		return
	}
	*into = found.Int64
	c.sampled++
}
