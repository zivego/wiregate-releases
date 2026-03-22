export async function copyText(value: string): Promise<void> {
  if (typeof navigator === "undefined" || !navigator.clipboard) {
    throw new Error("clipboard API unavailable");
  }
  await navigator.clipboard.writeText(value);
}
