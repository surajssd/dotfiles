// Make fs.chmod/chmodSync best-effort on /home/node/.openclaw and below.
// Works around an upstream regression where exec-approvals.ts:ensureDir
// chmods the bind-mount root on every exec invocation. macOS VirtioFS
// rejects chmod on bind-mount roots with EPERM, killing the exec tool.
// Upstream's other state-dir chmods already swallow errors; this aligns
// exec-approvals' chmod with that pattern, scoped to our bind path only.
const fs = require("node:fs");

const SCOPED_PREFIX = "/home/node/.openclaw";

function isScoped(p) {
    if (typeof p !== "string") return false;
    return p === SCOPED_PREFIX || p.startsWith(SCOPED_PREFIX + "/");
}

const origChmodSync = fs.chmodSync;
fs.chmodSync = function (path, mode) {
    try {
        return origChmodSync.call(this, path, mode);
    } catch (err) {
        if (err && err.code === "EPERM" && isScoped(path)) return undefined;
        throw err;
    }
};

const origChmod = fs.promises.chmod;
fs.promises.chmod = async function (path, mode) {
    try {
        return await origChmod.call(this, path, mode);
    } catch (err) {
        if (err && err.code === "EPERM" && isScoped(path)) return undefined;
        throw err;
    }
};
