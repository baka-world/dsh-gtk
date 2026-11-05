// dsh-gtk binds the port and passes the listening socket to dsh as fd 3, so
// dsh cannot fail to bind it — and the web UI keeps one origin across starts,
// which is what its localStorage (sidebar width, selected session) is keyed by.
//
// The shell starts node with this file as a --require preload and sets
// DSH_GTK_LISTEN_FD / DSH_GTK_LISTEN_PORT. The first server.listen() on that
// port (the webserver plugin's) adopts the fd instead of binding a new socket;
// every other listen call is left alone.
const fd = Number(process.env.DSH_GTK_LISTEN_FD || 0);
const port = Number(process.env.DSH_GTK_LISTEN_PORT || -1);
if (fd > 0 && port >= 0) {
  // dsh starts child processes of its own; none of them should see this.
  delete process.env.DSH_GTK_LISTEN_FD;
  delete process.env.DSH_GTK_LISTEN_PORT;

  const net = require('node:net');
  const listen = net.Server.prototype.listen;
  let adopted = false;

  net.Server.prototype.listen = function (...args) {
    if (!adopted && args[0] === port) {
      adopted = true;
      const callback = typeof args[args.length - 1] === 'function' ? args[args.length - 1] : undefined;
      return listen.call(this, { fd }, callback);
    }
    return listen.apply(this, args);
  };
}
