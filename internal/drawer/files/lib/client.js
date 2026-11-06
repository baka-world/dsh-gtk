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

/* With the drawer closed the sidebar is off screen, and the toggle lives inside
   it: this handle is the way back. */
[data-dsh-gtk-drawer-handle] {
  position: fixed;
  top: 10px;
  left: 10px;
  z-index: 60;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: var(--dsw-alias-bg-base, #ffffff);
  color: var(--dsw-alias-label-secondary, #4b4b4b);
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.18);
  cursor: pointer;
}

[data-dsh-gtk-drawer-handle]:hover {
  background: var(--dsw-alias-bg-hover, #f0f0f0);
}

@media (prefers-reduced-motion: reduce) {
  [class$="_sidebarCol"] { transition: none; }
}
`

    const HANDLE_ICON =
      '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" ' +
      'stroke-width="1.5" stroke-linecap="round"><path d="M2.5 4h11M2.5 8h11M2.5 12h11"/></svg>'

    // The frame renders after the plugin applies, so wait for it.
    function installHandle() {
      const frame = document.querySelector('div[style*="grid-template-columns"]')
      if (frame === null) return false
      if (document.querySelector('[data-dsh-gtk-drawer-handle]') !== null) return true

      const handle = document.createElement('button')
      handle.type = 'button'
      handle.setAttribute('data-dsh-gtk-drawer-handle', '')
      handle.setAttribute('aria-label', '打开侧边栏')
      handle.innerHTML = HANDLE_ICON
      handle.addEventListener('click', () => {
        const toggle = document.querySelector('[class$="_sidebarCol"] button[aria-label]')
        if (toggle !== null) toggle.click()
      })
      document.body.appendChild(handle)

      const sync = () => {
        handle.style.display = frame.hasAttribute('data-sidebar-collapsed') ? 'flex' : 'none'
      }
      new MutationObserver(sync).observe(frame, {
        attributes: true,
        attributeFilter: ['data-sidebar-collapsed'],
      })
      sync()
      return true
    }

    function apply() {
      const style = document.createElement('style')
      style.setAttribute('data-dsh-gtk-drawer', '')
      style.textContent = CSS
      document.head.appendChild(style)

      if (!installHandle()) {
        const timer = setInterval(() => {
          if (installHandle()) clearInterval(timer)
        }, 200)
      }
    }

    exports.apply = apply
    exports.inject = inject
    return module.exports
  }
})
