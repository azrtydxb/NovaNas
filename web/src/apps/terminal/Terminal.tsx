// Terminal — design-faithful skeleton. Backend PTY not yet wired.

export function Terminal() {
  return (
    <div className="app-term">
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span> zpool status
      </div>
      <div className="term-out">
        {"  pool: fast"}
        <br />
        {" state: ONLINE"}
        <br />
        {"  scan: scrub repaired 0B in 2h14m"}
      </div>
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span> kubectl get pods -n apps
      </div>
      <div className="term-out">
        NAME              READY   STATUS    RESTARTS   AGE
        <br />
        immich-app-0      1/1     Running   0          2d
        <br />
        immich-worker-0   1/1     Running   0          2d
        <br />
        plex-1            1/1     Running   0          12d
      </div>
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span>{" "}
        <span className="term-cursor">_</span>
      </div>
    </div>
  );
}

export default Terminal;
