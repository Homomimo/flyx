import type { InboundApiItem, InboundRelayApiItem } from "@/api/types";

import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";

import { getInboundList, getInboundRelays } from "@/api";
import { AnimatedPage } from "@/components/animated-page";
import { PageLoadingState } from "@/components/page-state";
import {
  EmptyHint,
  ListRow,
  PageHeader,
  PageTabs,
  PageToolbar,
} from "@/components/page-kit";
import ForwardPage from "@/pages/forward";
import { RelayModal } from "@/pages/inbound/relay-modal";
import TunnelPage from "@/pages/tunnel";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Chip } from "@/shadcn-bridge/heroui/chip";

type RelayTab = "lines" | "tunnels" | "rules";

const TAB_KEY = "flvx_relay_tab";

function readTab(): RelayTab {
  try {
    const v = localStorage.getItem(TAB_KEY);

    if (v === "lines" || v === "tunnels" || v === "rules") return v;
  } catch {
    // Remembering the tab is only a convenience.
  }

  return "lines";
}

const TABS: Array<{ key: RelayTab; label: string }> = [
  { key: "lines", label: "线路" },
  { key: "tunnels", label: "隧道" },
  { key: "rules", label: "规则" },
];

/** Relay lines grouped by the landing they lead to. */
function LinesView() {
  const [loading, setLoading] = useState(true);
  const [inbounds, setInbounds] = useState<InboundApiItem[]>([]);
  const [relays, setRelays] = useState<InboundRelayApiItem[]>([]);
  const [editing, setEditing] = useState<InboundApiItem | null>(null);
  const [onlyWithRelays, setOnlyWithRelays] = useState(false);
  const [query, setQuery] = useState("");

  const load = useCallback(async () => {
    try {
      const [inRes, relRes] = await Promise.all([
        getInboundList(),
        getInboundRelays(0),
      ]);

      if (inRes.code === 0) setInbounds(inRes.data ?? []);
      if (relRes.code === 0) setRelays(relRes.data ?? []);
      if (inRes.code !== 0) toast.error(inRes.msg || "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const byInbound = useMemo(() => {
    const m = new Map<number, InboundRelayApiItem[]>();

    for (const r of relays) {
      m.set(r.inboundId, [...(m.get(r.inboundId) ?? []), r]);
    }

    return m;
  }, [relays]);

  if (loading) return <PageLoadingState message="正在加载..." />;

  const q = query.trim().toLowerCase();
  const shown = inbounds.filter(
    (i) =>
      (!onlyWithRelays || (byInbound.get(i.id)?.length ?? 0) > 0) &&
      (!q || i.name.toLowerCase().includes(q)),
  );

  return (
    <div className="space-y-3">
      <PageToolbar
        placeholder="搜索落地名称"
        search={query}
        onSearch={setQuery}
      >
        <Button
          className="shrink-0"
          color={onlyWithRelays ? "primary" : "default"}
          size="sm"
          variant="flat"
          onPress={() => setOnlyWithRelays((v) => !v)}
        >
          只看有中转的
        </Button>
      </PageToolbar>

      {shown.length === 0 ? (
        <EmptyHint
          text={
            inbounds.length === 0
              ? "还没有落地。先在“落地”里搭建或导入节点，再回来对接中转"
              : query
                ? "没有匹配的落地"
                : "还没有中转线路"
          }
        />
      ) : (
        <div className="grid grid-cols-1 gap-2 2xl:grid-cols-2">
          {shown.map((i) => {
            const lines = byInbound.get(i.id) ?? [];
            const broken = lines.some(
              (l) => !(l.forwardExists && l.forwardStatus === 1),
            );

            return (
              <ListRow
                key={i.id}
                badges={
                  i.relayOnly && (
                    <Chip className="shrink-0" size="sm" variant="flat">
                      仅中转
                    </Chip>
                  )
                }
                meta={
                  lines.length > 0
                    ? lines
                        .map((l) => {
                          const entry = l.entries[0];

                          return `${l.name || l.tunnelName}${
                            entry
                              ? ` ${l.shareHost || entry.Host}:${entry.Port}`
                              : ""
                          }`;
                        })
                        .join(" · ")
                    : `${i.external ? "导入的节点" : i.nodeName} · 无中转，点击添加`
                }
                right={
                  <span className="text-xs text-default-500">
                    {lines.length > 0 ? `${lines.length} 条` : ""}
                  </span>
                }
                status={lines.length === 0 ? "off" : broken ? "warn" : "ok"}
                statusTitle={
                  lines.length === 0
                    ? "无中转"
                    : broken
                      ? "有中转转发已暂停或被删除"
                      : "中转正常"
                }
                title={i.name}
                onPress={() => setEditing(i)}
              />
            );
          })}
        </div>
      )}

      <RelayModal
        item={editing}
        onChanged={load}
        onClose={() => setEditing(null)}
      />
    </div>
  );
}

/** 中转: relay lines per landing, plus the tunnel and rule pages. */
export default function RelayPage() {
  const [tab, setTabState] = useState<RelayTab>(readTab);
  const setTab = (t: RelayTab) => {
    setTabState(t);
    try {
      localStorage.setItem(TAB_KEY, t);
    } catch {
      // ignore
    }
  };

  return (
    <AnimatedPage className="px-3 py-6 lg:px-6">
      <PageHeader
        description="让客户端先连中转再到落地：线路按落地管理，隧道和规则是底层的转发配置"
        title="中转"
      />
      <PageTabs tabs={TABS} value={tab} onChange={setTab} />
      {tab === "lines" ? (
        <LinesView />
      ) : tab === "tunnels" ? (
        <TunnelPage embedded />
      ) : (
        <ForwardPage embedded />
      )}
    </AnimatedPage>
  );
}
