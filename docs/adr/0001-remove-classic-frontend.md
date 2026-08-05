# Remove the classic frontend theme (web/classic)

We removed the `web/classic` legacy frontend (React 18, Vite, Semi Design) and all runtime theme-switching machinery, leaving the `web/default` frontend (React 19, Rsbuild, Base UI) as the only shipped UI. The default frontend already covers every classic page and feature — same backend API, same task/Midjourney log data, same operations — so classic was a redundant second UI, not a style skin of default; its build chain was known to break (date-fns/date-fns-tz) and two code TODOs already awaited its removal.

**Status:** accepted

**Consequences**

- Deployments whose database still holds `theme.frontend=classic` silently serve the default frontend after upgrade; the option is no longer read, validated, or returned by the API, and the theme selector was removed from the default frontend's System Information settings.
- The legacy `DELETE /api/log/` endpoint (classic-only) was removed; the default frontend uses `POST /api/system-task/log-cleanup`.
- The old `/console/*` → modern path rewrites (`/console/topup`→`/wallet`, `/console/log`→`/usage-logs`, `/console/personal`→`/profile`) are preserved as `common.ModernizePath`, now applied unconditionally (they were previously gated on the default theme).
- Release builds (Docker + CI) no longer compile or embed the classic frontend, shrinking the binary and removing a build prerequisite.
