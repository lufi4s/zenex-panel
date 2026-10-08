/** Joins class names, skipping empty values. Pass classes in the order you want them applied. */
export function cn(...inputs: Array<string | false | null | undefined>): string {
  return inputs.filter(Boolean).join(" ");
}
