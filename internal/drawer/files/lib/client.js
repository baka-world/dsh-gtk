// The sidebar as an overlay drawer (see README.md): the grid keeps no track for
// it, so the centre column has one width whether the sidebar is open or closed,
// and a toggle animates a transform on the compositor instead of reflowing the
// transcript on every frame.
//
// Nothing here depends on dsh's hashed class names: the frame and the sidebar
// are found from the toggle button's position in the DOM and tagged with our
// own attributes, which is what the stylesheet targets.
window.__ModuleLoader__.load({
  id: 'dsh-gtk-drawer',
  factory: () => {
    const module = { exports: {} }
    const exports = module.exports
    const inject = []

    const FRAME = 'data-dsh-gtk-drawer-frame'
    const SIDEBAR = 'data-dsh-gtk-drawer-sidebar'
    const HANDLE = 'data-dsh-gtk-drawer-handle'

    const CSS = `
/* The frame keeps no track for the sidebar: the centre column does not change
   width when the sidebar opens or closes. */
[${FRAME}] {
  grid-template-columns: 0px minmax(0px, 1fr) minmax(0px, 0px) !important;
  transition: none !important;
}

/* The sidebar floats above the transcript and slides, which the compositor can
   do without the main thread. Its width follows the one dsh had set for it. */
[${SIDEBAR}] {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  /* !important: collapsed, dsh shrinks the sidebar to an icon rail, and
     translateX(-100%) would then move it out by the rail's width only. */
  width: var(--dsh-gtk-drawer-width, 280px) !important;
  z-index: 40;
  transform: translateX(-100%);
  transition: transform 180ms var(--ds-ease-in-out, ease);
  will-change: transform;
  box-shadow: var(--dsh-gtk-drawer-shadow, 0 0 24px rgb(0 0 0 / 0.18));
}

[${FRAME}]:not([data-sidebar-collapsed]) > [${SIDEBAR}] {
  transform: none;
}

/* With the drawer closed the sidebar is off screen, and the toggle lives inside
   it: this handle is the way back. */
[${HANDLE}] {
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

[${HANDLE}]:hover {
  background: var(--dsw-alias-bg-hover, #f0f0f0);
}

[${HANDLE}] {
  opacity: 0;
  transition: opacity 140ms var(--ds-ease-in-out, ease);
}

[${HANDLE}][data-open] {
  opacity: 1;
}

@media (prefers-reduced-motion: reduce) {
  [${SIDEBAR}] { transition: none; }
}
`

    // The icon dsh's own toggle shows when the sidebar is collapsed.
    const HANDLE_ICON =
      '<svg width="16" height="16" aria-hidden="true">' +
      '<use href="#dsw-icon-panel-left-outline-regular"></use></svg>'

    const TOGGLE = /侧边栏|sidebar/i

    /** The sidebar's toggle, wherever dsh rendered it. */
    function toggle() {
      const all = document.querySelectorAll('button[aria-label]')
      for (const button of all) {
        if (TOGGLE.test(button.getAttribute('aria-label')) && button.getClientRects().length > 0) {
          return button
        }
      }
      for (const button of all) {
        if (TOGGLE.test(button.getAttribute('aria-label'))) return button
      }
      return null
    }

    /** The sidebar column and the grid frame that holds it, found from the
     *  toggle: the sidebar is the ancestor that is a direct child of a grid
     *  container sized by an inline grid-template-columns. */
    function locate(button) {
      let node = button
      while (node && node !== document.body) {
        const parent = node.parentElement
        if (parent && parent.style && /grid-template-columns/.test(parent.style.cssText || '')) {
          return { sidebar: node, frame: parent }
        }
        node = parent
      }
      return null
    }

    /** dsh writes the sidebar's width as the frame's first grid track; keep the
     *  drawer on it, so a width the reader dragged survives the drawer. */
    function followWidth(frame, sidebar) {
      // Collapsed, dsh writes the icon rail's width into the first track: that
      // is not the drawer's width, and taking it would slide the drawer out by
      // 56px instead of its own width.
      if (frame.hasAttribute('data-sidebar-collapsed')) return
      const match = /grid-template-columns:\s*([\d.]+)px/.exec(frame.style.cssText || '')
      if (match === null) return
      const width = Number(match[1])
      if (!(width > 0)) return
      // The variable goes on the sidebar itself: setting it on the frame would
      // invalidate the style of everything inside it, which is the cost this
      // plugin exists to avoid.
      const value = width + 'px'
      if (sidebar.style.getPropertyValue('--dsh-gtk-drawer-width') !== value) {
        sidebar.style.setProperty('--dsh-gtk-drawer-width', value)
      }
    }

    function install() {
      const button = toggle()
      if (button === null) return false
      const found = locate(button)
      if (found === null) return false

      found.frame.setAttribute(FRAME, '')
      found.sidebar.setAttribute(SIDEBAR, '')
      followWidth(found.frame, found.sidebar)
      ensureHandle(found.frame)
      // The reader can drag the sidebar's edge: dsh rewrites the frame's first
      // track, and the drawer follows it.
      if (found.frame.__widthWatch !== true) {
        found.frame.__widthWatch = true
        new MutationObserver(() => followWidth(found.frame, found.sidebar)).observe(found.frame, {
          attributes: true,
          attributeFilter: ['style'],
        })
      }
      return true
    }

    function ensureHandle(frame) {
      let handle = document.querySelector('[' + HANDLE + ']')
      if (handle === null) {
        handle = document.createElement('button')
        handle.type = 'button'
        handle.setAttribute(HANDLE, '')
        handle.setAttribute('aria-label', '打开侧边栏')
        handle.innerHTML = HANDLE_ICON
        handle.addEventListener('click', () => {
          const button = toggle()
          if (button !== null) button.click()
        })
        document.body.appendChild(handle)
      }
      const sync = () => {
        const open = frame.hasAttribute('data-sidebar-collapsed')
        handle.style.display = 'flex'
        if (open) handle.setAttribute('data-open', '')
        else handle.removeAttribute('data-open')
      }
      if (handle.__sync !== sync) {
        handle.__sync = sync
        new MutationObserver(sync).observe(frame, {
          attributes: true,
          attributeFilter: ['data-sidebar-collapsed', 'style'],
        })
      }
      sync()
    }

    // A re-render can replace the frame or drop our attributes. Watching every
    // mutation of the page costs a query per frame on a busy transcript, so this
    // only looks once a second, and only when the tags are gone.
    function watch() {
      setInterval(() => {
        if (document.querySelector('[' + FRAME + ']') === null ||
            document.querySelector('[' + SIDEBAR + ']') === null) {
          install()
        }
      }, 1000)
    }

    function apply() {
      const style = document.createElement('style')
      style.setAttribute('data-dsh-gtk-drawer', '')
      style.textContent = CSS
      document.head.appendChild(style)

      if (!install()) {
        const timer = setInterval(() => {
          if (install()) clearInterval(timer)
        }, 200)
      }
      watch()
    }

    exports.apply = apply
    exports.inject = inject
    return module.exports
  }
})
