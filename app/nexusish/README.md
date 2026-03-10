# Nexusish

Terminal dashboard for administering a Nexus gateway.

Key packages:

- `runtime`: Nexus admin client and state loading.
- `tui`: Bubble Tea UI, pane components, and dashboard screens.

The TUI consumes the shared Nexus admin contract types from `app/nexus/adminapi`
and renders them through local view-oriented pane models.
