import type { UserSubscription } from "@/api";

import { useCallback, useEffect, useState } from "react";
import toast from "react-hot-toast";
import { ChevronDown, Copy, QrCode, RefreshCw } from "lucide-react";

import { getMySubscriptions, resetMySubscription } from "@/api";
import { AnimatedPage } from "@/components/animated-page";
import { EmptyHint, PageHeader } from "@/components/page-kit";
import { PageLoadingState } from "@/components/page-state";
import {
  markPageCached,
  pageCached,
  useCachedState,
} from "@/hooks/use-cached-state";
import { QrImage, copyText, panelOrigin } from "@/pages/inbound/share";
import { SubClientGrid } from "@/pages/subscription/client-grid";
import { SubTrafficStats } from "@/components/lazy-charts";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { formatTraffic } from "@/utils/traffic";

const DAY = 86400000;

const link = (s: UserSubscription, format = "") =>
  `${panelOrigin()}${s.path}${format ? `?format=${format}` : ""}`;

function ymd(ms: number) {
  const d = new Date(ms);

  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** The status pill: the blocking state, else a warning, else 正常. */
function status(s: UserSubscription): [string, string] {
  const used = s.uploadBytes + s.downloadBytes;
  const left = s.expireTime ? s.expireTime - Date.now() : Infinity;

  switch (s.status) {
    case "disabled":
      return ["已停用", "text-default-400"];
    case "expired":
      return ["已到期", "text-danger"];
    case "over_quota":
      return ["流量已用完", "text-danger"];
  }
  if (s.trafficLimit && used >= s.trafficLimit * 0.9) {
    return ["流量快用完", "text-warning-600"];
  }
  if (left < 3 * DAY) return ["快到期", "text-warning-600"];

  return ["正常", "text-default-400"];
}

function expiryText(s: UserSubscription) {
  if (!s.expireTime) return "永不过期";
  const days = Math.ceil((s.expireTime - Date.now()) / DAY);

  if (days <= 0) return "已到期";
  if (days <= 7) return `剩 ${days} 天`;

  return ymd(s.expireTime);
}

function Traffic({ s }: { s: UserSubscription }) {
  const used = s.uploadBytes + s.downloadBytes;
  const pct = s.trafficLimit ? Math.min(100, (used / s.trafficLimit) * 100) : 0;
  const tone =
    pct >= 100 ? "bg-danger" : pct >= 90 ? "bg-warning" : "bg-foreground";

  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-2xl font-semibold tabular-nums tracking-tight">
          {formatTraffic(used)}
          <span className="ml-1 text-sm font-normal text-default-400">
            {s.trafficLimit ? `/ ${formatTraffic(s.trafficLimit)}` : "不限流量"}
          </span>
        </span>
        {s.trafficLimit > 0 && (
          <span className="text-xs tabular-nums text-default-400">
            {Math.floor(pct)}%
          </span>
        )}
      </div>
      {s.trafficLimit > 0 && (
        <div className="h-[3px] overflow-hidden rounded-full bg-default-100">
          <div
            className={`h-full rounded-full ${tone}`}
            style={{ width: `${Math.max(2, pct)}%` }}
          />
        </div>
      )}
      <div className="text-xs tabular-nums text-default-400">
        {ymd(s.trafficSince)} 起 · ↑ {formatTraffic(s.uploadBytes)} · ↓{" "}
        {formatTraffic(s.downloadBytes)}
        {s.nextResetTime ? ` · ${ymd(s.nextResetTime)} 重置` : ""}
      </div>
    </div>
  );
}

function SubscriptionCard({
  s,
  open,
  onToggle,
  onQr,
  onReset,
}: {
  s: UserSubscription;
  /** Whether the import section shows (collapsible when there are several). */
  open: boolean;
  onToggle?: () => void;
  onQr: () => void;
  onReset: () => void;
}) {
  const [label, tone] = status(s);
  const usable = s.status === "ok";
  const [traffic, setTraffic] = useState(false);

  return (
    <section className="space-y-4 py-6 first:pt-0">
      <div>
        <div className="flex items-baseline justify-between gap-2">
          <h2 className="min-w-0 truncate text-base font-semibold">{s.name}</h2>
          <span className={`shrink-0 text-xs ${tone}`}>{label}</span>
        </div>
        <p className="mt-0.5 text-xs text-default-400">
          {s.nodeCount} 个节点 · {expiryText(s)}
          {s.expireTime && expiryText(s) === ymd(s.expireTime) ? " 到期" : ""}
          {" · "}
          {s.resetDay ? `每月 ${s.resetDay} 号重置` : "不自动重置"}
        </p>
      </div>
      <Traffic s={s} />
      <button
        aria-expanded={traffic}
        className="flex w-full items-center justify-between border-t border-divider pt-3 text-sm text-default-500 hover:text-foreground"
        type="button"
        onClick={() => setTraffic(!traffic)}
      >
        流量统计
        <ChevronDown
          className={`h-4 w-4 transition-transform ${traffic ? "rotate-180" : ""}`}
        />
      </button>
      {traffic && <SubTrafficStats mine id={s.id} />}
      {!usable && (
        <p className="text-xs text-danger">
          {s.status === "over_quota"
            ? "流量已用完，暂时连不上；到重置日或联系管理员恢复"
            : s.status === "expired"
              ? "订阅已到期，请联系管理员续期"
              : "订阅已被管理员停用"}
        </p>
      )}
      {onToggle && (
        <button
          className="flex w-full items-center justify-between border-t border-divider pt-3 text-sm text-default-500 hover:text-foreground"
          type="button"
          onClick={onToggle}
        >
          导入客户端
          <ChevronDown
            className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`}
          />
        </button>
      )}
      {open && (
        <div className="space-y-3">
          {!onToggle && (
            <div className="border-t border-divider pt-3 text-sm text-default-500">
              导入客户端
            </div>
          )}
          <SubClientGrid
            configClients={s.templates}
            link={(format) => link(s, format)}
            name={s.name}
            rules={s.rules}
          />
          <p className="text-xs text-default-400">
            点客户端直接导入
            <span className="hidden sm:inline">
              ，右侧按钮复制该客户端的链接
            </span>
          </p>
        </div>
      )}
      <div className="flex gap-2">
        <Button
          className="flex-1"
          color="primary"
          size="sm"
          startContent={<Copy className="h-3.5 w-3.5" />}
          onPress={() => copyText(link(s), "订阅链接已复制")}
        >
          复制订阅链接
        </Button>
        <Button
          isIconOnly
          aria-label="二维码"
          size="sm"
          title="二维码"
          variant="bordered"
          onPress={onQr}
        >
          <QrCode className="h-4 w-4" />
        </Button>
        <Button
          className="text-danger"
          size="sm"
          startContent={<RefreshCw className="h-3.5 w-3.5" />}
          variant="light"
          onPress={onReset}
        >
          重置链接
        </Button>
      </div>
    </section>
  );
}

/** 订阅 for regular users: the subscriptions an admin assigned to them. */
// A revisit shows the last data at once and reloads in the background.
const PAGE = "my-subscriptions";

export default function MySubscriptionsPage() {
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useCachedState<UserSubscription[]>(
    PAGE,
    "items",
    [],
  );
  const [openId, setOpenId] = useState<number | null>(null);
  const [qr, setQr] = useState<UserSubscription | null>(null);
  const [resetting, setResetting] = useState<UserSubscription | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await getMySubscriptions();

      if (res.code === 0) {
        const list = res.data ?? [];

        setItems(list);
        setOpenId((prev) => prev ?? list[0]?.id ?? null);
        markPageCached(PAGE);
      } else {
        toast.error(res.msg || "加载失败");
      }
    } catch {
      toast.error("加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const reset = async () => {
    if (!resetting) return;
    setBusy(true);
    try {
      const res = await resetMySubscription(resetting.id);

      if (res.code !== 0 || !res.data) {
        toast.error(res.msg || "重置失败");

        return;
      }
      const next = res.data;

      setItems((prev) => prev.map((s) => (s.id === next.id ? next : s)));
      if (res.msg && res.msg !== "操作成功") toast(res.msg, { icon: "⚠️" });
      else toast.success("已重置，请重新导入客户端");
      setResetting(null);
    } catch {
      toast.error("重置失败");
    } finally {
      setBusy(false);
    }
  };

  if (loading && !pageCached(PAGE))
    return <PageLoadingState message="正在加载..." />;

  const several = items.length > 1;

  return (
    <AnimatedPage className="mx-auto max-w-3xl px-4 py-6 lg:px-10 lg:py-8">
      <PageHeader
        description="管理员分配给你的订阅：一键导入客户端，查看流量和到期时间"
        title="我的订阅"
      />
      {items.length === 0 ? (
        <EmptyHint text="还没有分配给你的订阅，请联系管理员" />
      ) : (
        <div className="divide-y divide-divider">
          {items.map((s) => (
            <SubscriptionCard
              key={s.id}
              open={!several || openId === s.id}
              s={s}
              onQr={() => setQr(s)}
              onReset={() => setResetting(s)}
              onToggle={
                several
                  ? () => setOpenId((id) => (id === s.id ? null : s.id))
                  : undefined
              }
            />
          ))}
        </div>
      )}

      <Modal
        isOpen={Boolean(qr)}
        placement="center"
        size="sm"
        onOpenChange={(v) => !v && setQr(null)}
      >
        <ModalContent>
          {() => (
            <>
              <ModalHeader closable>{qr?.name}</ModalHeader>
              <ModalBody className="items-center">
                {qr && <QrImage text={link(qr)} />}
                <p className="text-center text-xs text-default-500">
                  用客户端扫码导入（自动识别客户端格式）
                </p>
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        isOpen={Boolean(resetting)}
        placement="center"
        size="sm"
        onOpenChange={(v) => !v && setResetting(null)}
      >
        <ModalContent>
          {(close) => (
            <>
              <ModalHeader>重置订阅链接</ModalHeader>
              <ModalBody>
                <p className="text-sm">
                  「{resetting?.name}」会换成新的链接和凭据，
                  <b>旧链接和所有设备上已导入的配置都会立即失效</b>
                  ，需要重新导入。适合链接泄露时使用。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={close}>
                  取消
                </Button>
                <Button color="danger" isLoading={busy} onPress={reset}>
                  重置
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </AnimatedPage>
  );
}
