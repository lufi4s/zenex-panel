import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient, resetQueryCache, useQuery } from "./query";

afterEach(() => resetQueryCache());

describe("useQuery", () => {
  it("fetches again when the data is invalidated while a fetch is running", async () => {
    let calls = 0;
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));

    const query = createRoot(() =>
      useQuery({
        queryKey: ["invalidate-during-fetch"],
        queryFn: async () => {
          calls += 1;
          // The first read is slow, so it may return data from before the change.
          if (calls === 1) await gate;
          return calls;
        },
      }),
    );

    await queryClient.invalidateQueries({ queryKey: ["invalidate-during-fetch"] });
    release();

    await vi.waitFor(() => expect(query.data).toBe(2));
    expect(calls).toBe(2);
  });

  it("does not fetch twice for one invalidation when nothing is running", async () => {
    let calls = 0;
    const query = createRoot(() =>
      useQuery({
        queryKey: ["invalidate-idle"],
        queryFn: async () => {
          calls += 1;
          return calls;
        },
      }),
    );
    await vi.waitFor(() => expect(query.data).toBe(1));

    await queryClient.invalidateQueries({ queryKey: ["invalidate-idle"] });
    await vi.waitFor(() => expect(query.data).toBe(2));
    expect(calls).toBe(2);
  });
});
