import { describe, expect, it } from "vitest";
import { type KeyAction, keyAction } from "./keys";

const ev = (key: string, mods: Partial<Record<"ctrlKey" | "metaKey" | "altKey" | "shiftKey", boolean>> = {}) => ({
  type: "keydown",
  key,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

describe("keyAction", () => {
  const cases: [string, ReturnType<typeof ev>, boolean, boolean, KeyAction][] = [
    ["linux ctrl-c with selection copies", ev("c", { ctrlKey: true }), true, false, "copy"],
    ["linux ctrl-c without selection is SIGINT", ev("c", { ctrlKey: true }), false, false, "pass"],
    ["linux ctrl-v pastes", ev("v", { ctrlKey: true }), false, false, "paste"],
    ["linux ctrl-shift-c copies without selection state", ev("C", { ctrlKey: true, shiftKey: true }), false, false, "copy"],
    ["linux ctrl-shift-v pastes", ev("V", { ctrlKey: true, shiftKey: true }), false, false, "paste"],
    ["linux plain c passes", ev("c"), true, false, "pass"],
    ["linux ctrl-alt-c passes", ev("c", { ctrlKey: true, altKey: true }), true, false, "pass"],
    ["mac cmd-c with selection copies", ev("c", { metaKey: true }), true, true, "copy"],
    ["mac cmd-c without selection passes", ev("c", { metaKey: true }), false, true, "pass"],
    ["mac cmd-v pastes", ev("v", { metaKey: true }), false, true, "paste"],
    ["mac ctrl-c with selection stays SIGINT", ev("c", { ctrlKey: true }), true, true, "pass"],
    ["mac ctrl-v passes", ev("v", { ctrlKey: true }), false, true, "pass"],
    ["other keys pass", ev("a", { ctrlKey: true }), true, false, "pass"],
    ["keyup passes", { ...ev("c", { ctrlKey: true }), type: "keyup" }, true, false, "pass"],
  ];
  it.each(cases)("%s", (_n, e, sel, mac, want) => {
    expect(keyAction(e, sel, mac)).toBe(want);
  });
});
