import { fmt } from "@/format";
import { fmt as f2 } from "~format";
import { total } from "@/cart/total";
export function show(n: number) { return fmt(n) + f2(total([n])); }
