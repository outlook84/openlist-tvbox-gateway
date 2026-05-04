import { useRef } from "react";

export function useStableRowKeys(prefix: string, length: number) {
  const keys = useRef<string[]>([]);
  const nextID = useRef(1);

  while (keys.current.length < length) {
    keys.current.push(`${prefix}-${nextID.current}`);
    nextID.current += 1;
  }
  if (keys.current.length > length) {
    keys.current.length = length;
  }

  function add() {
    const key = `${prefix}-${nextID.current}`;
    nextID.current += 1;
    keys.current.push(key);
    return key;
  }

  function remove(index: number) {
    keys.current.splice(index, 1);
  }

  return { keys: keys.current, add, remove };
}
