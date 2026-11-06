// The sidebar as an overlay drawer (see README.md): the grid keeps no track for
// it, the sidebar floats over the transcript and slides with a transform, so a
// toggle animates on the compositor instead of reflowing the whole transcript
// on every frame.
window.__ModuleLoader__.load({
  id: 'dsh-gtk-drawer',
  factory: () => {
    const module = { exports: {} }
    const exports = module.exports
    const inject = []

    const CSS = `
/* The drawer: the grid keeps no track for the sidebar, so the centre column
   has the same width whether the sidebar is open or closed. Toggling therefore
   changes no layout in the transcript, which is what made the animation cost a
   reflow of every rendered message per frame. */
div[style*="grid-template-columns"] {
  grid-template-columns: 0px minmax(0px, 1fr) minmax(0px, 0px) !important;
  transition: none !important;
}

/* The sidebar floats above the transcript and slides, which the compositor can
   do without the main thread. */
[class$="_sidebarCol"] {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  width: var(--dsh-gtk-drawer-width, 280px);
  z-index: 40;
  transform: translateX(-100%);
  transition: transform 180ms var(--ds-ease-in-out, ease);
  box-shadow: var(--dsh-gtk-drawer-shadow, 0 0 24px rgb(0 0 0 / 0.18));
}

div[style*="grid-template-columns"]:not([data-sidebar-collapsed]) [class$="_sidebarCol"] {
  transform: none;
}

@media (prefers-reduced-motion: reduce) {
  [class$="_sidebarCol"] { transition: none; }
}
`

    function apply() {
      const style = document.createElement('style')
      style.setAttribute('data-dsh-gtk-drawer', '')
      style.textContent = CSS
      document.head.appendChild(style)
    }

    exports.apply = apply
    exports.inject = inject
    return module.exports
  }
})
