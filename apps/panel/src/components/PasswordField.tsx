import { createSignal, Show } from "solid-js";
import { Check, Copy, Eye, EyeOff } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { generatePassword } from "@/lib/password";

/**
 * A password box with a button to show or hide what is typed, one that makes a strong random
 * password (and shows it, so it can be copied), and one that copies it.
 */
export function PasswordField(props: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const [visible, setVisible] = createSignal(false);
  const [copied, setCopied] = createSignal(false);

  const generate = () => {
    props.onChange(generatePassword());
    setVisible(true);
  };
  const copy = async () => {
    await navigator.clipboard.writeText(props.value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div class="space-y-1.5">
      <Label for={props.id}>{props.label}</Label>
      <div class="flex items-center gap-1">
        <Input
          id={props.id}
          type={visible() ? "text" : "password"}
          autocomplete="new-password"
          spellcheck={false}
          class={visible() ? "font-mono" : undefined}
          value={props.value}
          onInput={(e) => props.onChange(e.currentTarget.value)}
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label={visible() ? "Hide password" : "Show password"}
          onClick={() => setVisible(!visible())}
        >
          {visible() ? <EyeOff aria-hidden /> : <Eye aria-hidden />}
        </Button>
        <Show when={props.value}>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-label="Copy password"
            onClick={() => void copy()}
          >
            {copied() ? <Check aria-hidden /> : <Copy aria-hidden />}
          </Button>
        </Show>
        <Button type="button" variant="outline" size="sm" onClick={generate}>
          Generate
        </Button>
      </div>
    </div>
  );
}
