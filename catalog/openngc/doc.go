// Package openngc provides a [resolve.Provider] for the OpenNGC catalog of
// NGC/IC deep-sky objects.
//
// Like every other astrogo catalog provider, it fetches its data over the
// network rather than reading anything embedded at build time — [New]
// downloads and merges the two upstream OpenNGC source CSVs if
// remote.EnableDownloads(remote.OpenNGC, ...) has been called, reusing a
// local cache untouched when nothing has changed upstream (see the
// README's "Data downloads & offline usage"). Without that consent, or on
// any other fetch failure, [New] returns an empty, warning-logged
// provider.
package openngc
