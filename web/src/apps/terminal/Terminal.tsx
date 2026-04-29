// TODO: phase 3 — wire to backend terminal/PTY API
// Terminal shell: renders the design's monospace prompt with placeholder
// output. Backend wiring lands in a later phase.

export function Terminal() {
  return (
    <div className="app-term">
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span> uname -a
      </div>
      <div className="term-out">
        Linux novanas 6.6.0 #1 SMP NovaNAS x86_64 GNU/Linux
      </div>
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span> echo "Terminal — backend
        wiring in a later phase"
      </div>
      <div className="term-out">Terminal — backend wiring in a later phase</div>
      <div className="term-line">
        <span className="term-prompt">novanas:~$</span>{" "}
        <span className="term-cursor">_</span>
      </div>
    </div>
  );
}

export default Terminal;
