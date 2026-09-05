package remote

import (
	"log/slog"

	"github.com/TuSKan/astrogo/internal/logging"
)

// SetLogger installs the logger astrogo writes to, replacing the default.
// Passing nil restores that default.
//
// # What astrogo logs
//
// Two kinds of line, and they are deliberately not treated alike:
//
//   - [slog.LevelInfo] — progress a caller may want and does not need: a
//     kernel is downloading, an EOP table has loaded. Discarded by default.
//   - [slog.LevelWarn] — a result has silently degraded. Today there is one:
//     no Earth Orientation Parameters could be found for the epoch, so DUT1
//     and polar motion are zero and topocentric accuracy has dropped to about
//     an arcsecond. [github.com/TuSKan/astrogo/time.Time.EOP] has no error
//     return, so this line is the only notice a caller gets. Emitted by
//     default, on stderr.
//
// So the default is quiet about progress and not quiet about accuracy. To
// silence everything, pass a logger over [slog.DiscardHandler]; to see the
// progress lines, pass one at Info.
//
// # Why this lives in remote
//
// Because remote is where a caller already comes to configure I/O — this sits
// beside [EnableDownloads] and [SetOffline], and every message astrogo emits
// is about data it fetched or failed to fetch. The logger itself is held one
// layer down, in internal/logging, because remote imports astrogo/time and so
// time cannot import remote back; see that package for the detail. There is
// still exactly one public setter.
//
// # Process-wide
//
// One logger for the module, like the endpoint registry and the download
// consent flags beside it. astrogo takes no logger parameter anywhere: a
// library that threads one through every signature makes every caller carry
// it, and almost none of them want to.
func SetLogger(l *slog.Logger) { logging.Set(l) }
