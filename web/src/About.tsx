import { useEffect, useRef } from "react";

const REPO_URL = "https://github.com/bbushvt/stay";

// Native <dialog>: Esc closes it and focus returns to the terminal that had it.
export function About({ version, open, onClose }: { version: string; open: boolean; onClose: () => void }) {
  const dlg = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const d = dlg.current!;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  return (
    <dialog
      ref={dlg}
      className="about"
      onClose={onClose}
      onClick={(e) => e.target === dlg.current && onClose()}
    >
      <h2>STAY</h2>
      <p className="muted">Shells That Await You</p>
      <p>Version {version}</p>
      <p>
        Created by Bryan Bush
        <br />
        <a href={REPO_URL} target="_blank" rel="noopener noreferrer">
          github.com/bbushvt/stay
        </a>
      </p>
      <p className="muted">
        Released under the{" "}
        <a href={`${REPO_URL}/blob/main/LICENSE`} target="_blank" rel="noopener noreferrer">
          MIT License
        </a>
      </p>
      <button onClick={onClose}>Close</button>
    </dialog>
  );
}
