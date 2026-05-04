import { useCallback, useMemo, useState } from "react";

function createRowKey(prefix: string, nextID: number) {
  return `${prefix}-${nextID}`;
}

function createRowKeys(prefix: string, startID: number, length: number) {
  return Array.from({ length }, (_, index) => createRowKey(prefix, startID + index));
}

export function useStableRowKeys(prefix: string, length: number) {
  const [state, setState] = useState(() => ({
    keys: createRowKeys(prefix, 1, length),
    nextID: length + 1,
  }));

  const keys = useMemo(() => {
    if (state.keys.length >= length) return state.keys.slice(0, length);
    return [...state.keys, ...createRowKeys(prefix, state.nextID, length - state.keys.length)];
  }, [length, prefix, state.keys, state.nextID]);

  const nextID = state.nextID + Math.max(0, length - state.keys.length);

  const add = useCallback(() => {
    const key = createRowKey(prefix, nextID);
    setState({ keys: [...keys, key], nextID: nextID + 1 });
    return key;
  }, [keys, nextID, prefix]);

  const remove = useCallback((index: number) => {
    setState({ keys: keys.filter((_, keyIndex) => keyIndex !== index), nextID });
  }, [keys, nextID]);

  return { keys, add, remove };
}
