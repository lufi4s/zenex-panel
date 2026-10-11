/** Letters and digits without the look-alikes (0 O 1 l I), so a password is easy to read out and type. */
const ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789";

/**
 * A random password from the browser's secure random source. Every character is drawn with
 * rejection sampling, so no character is more likely than another.
 */
export function generatePassword(length = 20): string {
  const limit = 256 - (256 % ALPHABET.length);
  let out = "";
  while (out.length < length) {
    const bytes = new Uint8Array(length * 2);
    crypto.getRandomValues(bytes);
    for (const b of bytes) {
      if (b < limit && out.length < length) out += ALPHABET[b % ALPHABET.length];
    }
  }
  return out;
}
