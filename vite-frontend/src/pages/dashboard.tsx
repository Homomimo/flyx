import { useState } from "react";
import toast from "react-hot-toast";

import { copyWithToast } from "@/utils/clipboard";
import { AnimatedPage } from "@/components/animated-page";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
} from "@/shadcn-bridge/heroui/modal";
import { PageLoadingState } from "@/components/page-state";
import { AnnouncementBanner } from "@/pages/dashboard/components/announcement-banner";
import { AnnouncementModal } from "@/pages/dashboard/components/announcement-modal";
import { FlowChartCard } from "@/components/lazy-charts";
import { getAdminFlag, getSessionName } from "@/utils/session";
import { AdminOverview } from "@/pages/dashboard/admin-overview";
import {
  formatTraffic,
  formatFlowLimit,
  flowLimitBytes,
} from "@/utils/traffic";
import {
  useDashboardData,
  type DashboardForward as Forward,
  type DashboardUserTunnel as UserTunnel,
} from "@/pages/dashboard/use-dashboard-data";

interface AddressItem {
  id: number;
  ip: string;
  address: string;
  copying: boolean;
}

/** Admins get the health overview; regular users their traffic and rules. */
export default function DashboardPage() {
  return getAdminFlag() ? <AdminOverview /> : <UserDashboard />;
}

function UserDashboard() {
  const username = getSessionName() || "User";

  const {
    loading,
    userInfo,
    userTunnels,
    forwardList,
    statisticsFlows,
    isAdmin,
    announcement,
    isAnnouncementModalOpen,
    setIsAnnouncementModalOpen,
    dismissAnnouncementModal,
  } = useDashboardData();

  const [addressModalOpen, setAddressModalOpen] = useState(false);
  const [addressModalTitle, setAddressModalTitle] = useState("");
  const [addressList, setAddressList] = useState<AddressItem[]>([]);

  const formatFlow = formatTraffic;

  const formatNumber = (value: number): string => {
    // 99999 表示无限制
    if (value === 99999) {
      return "无限制";
    }

    return value.toString();
  };

  // 处理24小时流量统计数据
  const processFlowChartData = () => {
    // 生成最近24小时的时间数组（从当前小时往前推24小时）
    const now = new Date();
    const hours: string[] = [];

    for (let i = 23; i >= 0; i--) {
      const time = new Date(now.getTime() - i * 60 * 60 * 1000);
      const hourString = time.getHours().toString().padStart(2, "0") + ":00";

      hours.push(hourString);
    }

    // 创建数据映射
    const flowMap = new Map<string, number>();

    statisticsFlows.forEach((item) => {
      flowMap.set(item.time, item.flow || 0);
    });

    // 生成图表数据，没有数据的小时显示为0
    return hours.map((hour) => ({
      time: hour,
      flow: flowMap.get(hour) || 0,
      // 格式化显示用的流量值
      formattedFlow: formatFlow(flowMap.get(hour) || 0),
    }));
  };

  const getExpStatus = (expTime?: string | number) => {
    if (!expTime)
      return {
        color: "text-default-400",
        bg: "",
        text: "永久",
      };

    const now = new Date();
    const expDate = new Date(expTime);

    if (isNaN(expDate.getTime())) {
      return {
        color: "text-default-400",
        bg: "",
        text: "无效",
      };
    }

    if (expDate < now) {
      return {
        color: "text-danger",
        bg: "",
        text: "已过期",
      };
    }

    const diffTime = expDate.getTime() - now.getTime();
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays <= 7) {
      return {
        color: "text-danger",
        bg: "",
        text: `${diffDays}天后过期`,
      };
    } else if (diffDays <= 30) {
      return {
        color: "text-warning-600",
        bg: "",
        text: `${diffDays}天后过期`,
      };
    } else {
      return {
        color: "text-default-400",
        bg: "",
        text: `${diffDays}天后过期`,
      };
    }
  };

  const calculateUserTotalUsedFlow = (): number => {
    // 后端已按计费类型处理流量，前端直接使用入站+出站总和
    return (userInfo.inFlow || 0) + (userInfo.outFlow || 0);
  };

  const calculateUsagePercentage = (type: "flow" | "forwards"): number => {
    if (type === "flow") {
      const totalUsed = calculateUserTotalUsedFlow();
      const totalLimit = flowLimitBytes(userInfo.flow || 0, userInfo.flowMiB);

      // 无限制时返回0%
      if (userInfo.flow === 99999 && !userInfo.flowMiB) return 0;

      return totalLimit > 0 ? Math.min((totalUsed / totalLimit) * 100, 100) : 0;
    } else if (type === "forwards") {
      const totalUsed = forwardList.length;
      const totalLimit = userInfo.num || 0;

      // 无限制时返回0%
      if (userInfo.num === 99999) return 0;

      return totalLimit > 0 ? Math.min((totalUsed / totalLimit) * 100, 100) : 0;
    }

    return 0;
  };

  const getUsageColor = (percentage: number) => {
    if (percentage >= 90) return "bg-danger";
    if (percentage >= 70) return "bg-warning";

    return "bg-foreground";
  };

  /** A thin usage line; nothing to draw for an unlimited quota. */
  const renderProgressBar = (
    percentage: number,
    _size: "sm" | "md" = "md",
    isUnlimited: boolean = false,
  ) =>
    isUnlimited ? null : (
      <div className="h-[3px] w-full overflow-hidden rounded-full bg-default-100">
        <div
          className={`h-full rounded-full ${getUsageColor(percentage)}`}
          style={{ width: `${Math.min(percentage, 100)}%` }}
        />
      </div>
    );

  const calculateTunnelUsedFlow = (tunnel: UserTunnel): number => {
    if (!tunnel) return 0;
    const inFlow = tunnel.inFlow || 0;
    const outFlow = tunnel.outFlow || 0;

    // 后端已按计费类型处理流量，前端直接使用入站+出站总和
    return inFlow + outFlow;
  };

  const calculateTunnelFlowPercentage = (tunnel: UserTunnel): number => {
    const totalUsed = calculateTunnelUsedFlow(tunnel);
    const totalLimit = flowLimitBytes(tunnel.flow || 0, tunnel.flowMiB);

    // 无限制时返回0%
    if (tunnel.flow === 99999 && !tunnel.flowMiB) return 0;

    return totalLimit > 0 ? Math.min((totalUsed / totalLimit) * 100, 100) : 0;
  };

  const getTunnelUsedForwards = (tunnelId: number): number => {
    return forwardList.filter((forward) => forward.tunnelId === tunnelId)
      .length;
  };

  const calculateTunnelForwardPercentage = (tunnel: UserTunnel): number => {
    const totalUsed = getTunnelUsedForwards(tunnel.tunnelId);
    const totalLimit = tunnel.num || 0;

    // 无限制时返回0%
    if (tunnel.num === 99999) return 0;

    return totalLimit > 0 ? Math.min((totalUsed / totalLimit) * 100, 100) : 0;
  };

  const formatResetTime = (resetDay?: number): string => {
    if (resetDay === undefined || resetDay === null) return "";
    if (resetDay === 0) return "不重置";

    const now = new Date();
    const currentDay = now.getDate();

    let daysUntilReset: number;

    if (resetDay > currentDay) {
      daysUntilReset = resetDay - currentDay;
    } else if (resetDay < currentDay) {
      const nextMonth = new Date(
        now.getFullYear(),
        now.getMonth() + 1,
        resetDay,
      );
      const diffTime = nextMonth.getTime() - now.getTime();

      daysUntilReset = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    } else {
      daysUntilReset = 0;
    }

    if (daysUntilReset === 0) {
      return "今日重置";
    } else if (daysUntilReset === 1) {
      return "明日重置";
    } else {
      return `${daysUntilReset}天后重置`;
    }
  };

  const groupedForwards = () => {
    const groups: {
      [key: string]: { tunnelName: string; forwards: Forward[] };
    } = {};

    forwardList.forEach((forward) => {
      const tunnelName = forward.tunnelName || "未知隧道";

      if (!groups[tunnelName]) {
        groups[tunnelName] = {
          tunnelName,
          forwards: [],
        };
      }
      groups[tunnelName].forwards.push(forward);
    });

    return Object.values(groups);
  };

  const formatInAddress = (ipString: string, port: number): string => {
    if (!ipString) return "";

    const items = ipString
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item);

    if (items.length === 0) return "";

    // 检查第一项是否已经包含端口（格式：IP:端口）
    const firstItem = items[0];
    const hasPort = /:\d+$/.test(firstItem);

    if (hasPort) {
      // inIp 已经包含完整的 IP:Port 组合
      if (items.length === 1) {
        return items[0];
      }

      return `${items[0]} (+${items.length - 1}个)`;
    }

    // inIp 只包含IP，需要添加端口（兼容旧数据）
    if (!port) return "";

    if (items.length === 1) {
      const ip = items[0];

      if (ip.includes(":") && !ip.startsWith("[")) {
        return `[${ip}]:${port}`;
      } else {
        return `${ip}:${port}`;
      }
    }

    const firstIp = items[0];
    let formattedFirstIp: string;

    if (firstIp.includes(":") && !firstIp.startsWith("[")) {
      formattedFirstIp = `[${firstIp}]`;
    } else {
      formattedFirstIp = firstIp;
    }

    return `${formattedFirstIp}:${port} (+${items.length - 1}个)`;
  };

  const formatRemoteAddress = (remoteAddr: string): string => {
    if (!remoteAddr) return "";

    const addresses = remoteAddr
      .split(",")
      .map((addr) => addr.trim())
      .filter((addr) => addr);

    if (addresses.length === 0) return "";

    if (addresses.length === 1) {
      return addresses[0];
    }

    return `${addresses[0]} (+${addresses.length - 1})`;
  };

  const hasMultipleIps = (ipString: string): boolean => {
    if (!ipString) return false;
    const ips = ipString
      .split(",")
      .map((ip) => ip.trim())
      .filter((ip) => ip);

    return ips.length > 1;
  };

  const hasMultipleRemoteAddresses = (remoteAddr: string): boolean => {
    if (!remoteAddr) return false;
    const addresses = remoteAddr
      .split(",")
      .map((addr) => addr.trim())
      .filter((addr) => addr);

    return addresses.length > 1;
  };

  const showAddressModal = (ipString: string, port: number, title: string) => {
    if (!ipString) return;

    const items = ipString
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item);

    if (items.length <= 1) {
      copyToClipboard(formatInAddress(ipString, port));

      return;
    }

    // 检查是否已经包含端口
    const hasPort = /:\d+$/.test(items[0]);

    let formattedList: AddressItem[];

    if (hasPort) {
      // 已经包含完整的 IP:Port 组合，直接使用
      formattedList = items.map((item, index) => ({
        id: index,
        ip: item,
        address: item,
        copying: false,
      }));
    } else {
      // 只包含IP，需要添加端口
      formattedList = items.map((ip, index) => {
        let formattedAddress: string;

        if (ip.includes(":") && !ip.startsWith("[")) {
          formattedAddress = `[${ip}]:${port}`;
        } else {
          formattedAddress = `${ip}:${port}`;
        }

        return {
          id: index,
          ip: ip,
          address: formattedAddress,
          copying: false,
        };
      });
    }

    setAddressList(formattedList);
    setAddressModalTitle(`${title} (${items.length}个)`);
    setAddressModalOpen(true);
  };

  const showRemoteAddressModal = (remoteAddr: string, title: string) => {
    if (!remoteAddr) return;

    const addresses = remoteAddr
      .split(",")
      .map((addr) => addr.trim())
      .filter((addr) => addr);

    if (addresses.length <= 1) {
      copyToClipboard(remoteAddr);

      return;
    }

    const formattedList = addresses.map((address, index) => {
      return {
        id: index,
        ip: address,
        address: address,
        copying: false,
      };
    });

    setAddressList(formattedList);
    setAddressModalTitle(`${title} (${addresses.length}个)`);
    setAddressModalOpen(true);
  };

  const copyToClipboard = async (text: string) => {
    await copyWithToast(text);
  };

  const copyAddress = async (addressItem: AddressItem) => {
    try {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: true } : item,
        ),
      );
      await copyToClipboard(addressItem.address);
    } catch {
      toast.error("复制失败");
    } finally {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: false } : item,
        ),
      );
    }
  };

  const copyAllAddresses = async () => {
    if (addressList.length === 0) return;
    const allAddresses = addressList.map((item) => item.address).join("\n");

    await copyToClipboard(allAddresses);
  };

  const calculateForwardBillingFlow = (forward: Forward): number => {
    if (!forward) return 0;

    const inFlow = forward.inFlow || 0;
    const outFlow = forward.outFlow || 0;

    // 后端已按计费类型处理流量，前端直接使用入站+出站总和
    return inFlow + outFlow;
  };

  if (loading) {
    return (
      <div className="px-3 lg:px-6 flex-grow pt-2 lg:pt-4">
        <PageLoadingState message="正在加载数据..." />
      </div>
    );
  }

  return (
    <AnimatedPage className="px-4 py-6 lg:px-10 lg:py-8">
      <div className="mb-6 flex items-baseline justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-tight">概览</h1>
        <p className="truncate text-xs text-default-400">{username}</p>
      </div>
      {announcement && <AnnouncementBanner announcement={announcement} />}
      {announcement && (
        <AnnouncementModal
          announcement={announcement}
          isOpen={isAnnouncementModalOpen}
          onClose={() => setIsAnnouncementModalOpen(false)}
          onDontShowAgain={dismissAnnouncementModal}
        />
      )}

      <div className="mb-8 grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-divider bg-divider">
        <div className="bg-background px-4 py-4 lg:px-5">
          <div className="text-sm text-default-500">流量</div>
          <div className="mt-1 text-2xl font-semibold tabular-nums tracking-tight">
            {formatFlow(calculateUserTotalUsedFlow())}
            <span className="block text-sm font-normal text-default-400 sm:ml-1 sm:inline">
              / {formatFlowLimit(userInfo.flow, userInfo.flowMiB)}
            </span>
          </div>
          <div className="mt-2">
            {renderProgressBar(
              calculateUsagePercentage("flow"),
              "sm",
              userInfo.flow === 99999 && !userInfo.flowMiB,
            )}
          </div>
          <div className="mt-1.5 flex justify-between gap-2 text-xs text-default-400">
            <span>
              {userInfo.flow === 99999 && !userInfo.flowMiB
                ? "不限流量"
                : `已用 ${calculateUsagePercentage("flow").toFixed(1)}%`}
            </span>
            {userInfo.flowResetTime !== undefined &&
              userInfo.flowResetTime !== null && (
                <span className="truncate">
                  {formatResetTime(userInfo.flowResetTime)}
                </span>
              )}
          </div>
        </div>
        <div className="bg-background px-4 py-4 lg:px-5">
          <div className="text-sm text-default-500">规则</div>
          <div className="mt-1 text-2xl font-semibold tabular-nums tracking-tight">
            {forwardList.length}
            <span className="ml-1 text-sm font-normal text-default-400">
              / {formatNumber(userInfo.num || 0)}
            </span>
          </div>
          <div className="mt-2">
            {renderProgressBar(
              calculateUsagePercentage("forwards"),
              "sm",
              userInfo.num === 99999,
            )}
          </div>
          <div className="mt-1.5 text-xs text-default-400">
            {userInfo.num === 99999
              ? "不限数量"
              : `已用 ${calculateUsagePercentage("forwards").toFixed(1)}%`}
          </div>
        </div>
      </div>

      <FlowChartCard
        chartData={processFlowChartData()}
        formatFlow={formatFlow}
        statisticsFlowsCount={statisticsFlows.length}
      />

      {/* 隧道权限 */}
      {!isAdmin && (
        <section className="mb-8">
          <h2 className="flex items-baseline gap-2 border-b border-divider pb-2 text-sm font-semibold">
            隧道
            <span className="text-xs font-normal tabular-nums text-default-400">
              {userTunnels.length}
            </span>
          </h2>
          {userTunnels.length === 0 ? (
            <p className="py-6 text-sm text-default-400">暂无隧道权限</p>
          ) : (
            <div className="divide-y divide-divider">
              {userTunnels.map((tunnel) => {
                const exp = getExpStatus(tunnel.expTime);
                const unlimited = tunnel.flow === 99999 && !tunnel.flowMiB;

                return (
                  <div
                    key={tunnel.id}
                    className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:gap-6"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm">
                        {tunnel.tunnelName}
                      </div>
                      <div className="truncate text-xs text-default-400">
                        {tunnel.tunnelFlow === 1 ? "单向计费" : "双向计费"}
                        {" · "}
                        <span className={exp.color}>{exp.text}</span>
                        {tunnel.flowResetTime !== undefined &&
                          tunnel.flowResetTime !== null &&
                          ` · ${formatResetTime(tunnel.flowResetTime)}`}
                      </div>
                    </div>
                    <div className="w-full sm:w-44">
                      <div className="flex justify-between text-xs tabular-nums">
                        <span className="text-default-400">流量</span>
                        <span>
                          {formatFlow(calculateTunnelUsedFlow(tunnel))}
                          <span className="text-default-400">
                            {" "}
                            / {formatFlowLimit(tunnel.flow, tunnel.flowMiB)}
                          </span>
                        </span>
                      </div>
                      <div className="mt-1">
                        {renderProgressBar(
                          calculateTunnelFlowPercentage(tunnel),
                          "sm",
                          unlimited,
                        )}
                      </div>
                    </div>
                    <div className="w-full sm:w-32">
                      <div className="flex justify-between text-xs tabular-nums">
                        <span className="text-default-400">规则</span>
                        <span>
                          {getTunnelUsedForwards(tunnel.tunnelId)}
                          <span className="text-default-400">
                            {" "}
                            / {formatNumber(tunnel.num)}
                          </span>
                        </span>
                      </div>
                      <div className="mt-1">
                        {renderProgressBar(
                          calculateTunnelForwardPercentage(tunnel),
                          "sm",
                          tunnel.num === 99999,
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>
      )}

      {/* 规则 */}
      <section>
        <h2 className="flex items-baseline gap-2 border-b border-divider pb-2 text-sm font-semibold">
          规则
          <span className="text-xs font-normal tabular-nums text-default-400">
            {forwardList.length}
          </span>
        </h2>
        {groupedForwards().length === 0 ? (
          <p className="py-6 text-sm text-default-400">暂无规则配置</p>
        ) : (
          groupedForwards().map((group) => (
            <div key={group.tunnelName}>
              <div className="flex items-baseline gap-1.5 border-b border-divider pb-1.5 pt-5 text-xs">
                <span className="font-medium text-default-500">
                  {group.tunnelName}
                </span>
                <span className="tabular-nums text-default-400">
                  {group.forwards.length} 个规则
                </span>
              </div>
              <div className="divide-y divide-divider">
                {group.forwards.map((forward) => (
                  <div
                    key={forward.id}
                    className="flex items-center gap-3 py-3"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm">{forward.name}</div>
                      <div className="flex min-w-0 items-center gap-1.5 text-xs tabular-nums text-default-400">
                        <button
                          className={`min-w-0 truncate ${hasMultipleIps(forward.inIp) ? "underline decoration-dotted underline-offset-2 hover:text-foreground" : "cursor-default"}`}
                          disabled={!hasMultipleIps(forward.inIp)}
                          title={formatInAddress(forward.inIp, forward.inPort)}
                          type="button"
                          onClick={() =>
                            showAddressModal(
                              forward.inIp,
                              forward.inPort,
                              "入口地址",
                            )
                          }
                        >
                          {formatInAddress(forward.inIp, forward.inPort)}
                        </button>
                        <span className="shrink-0">→</span>
                        <button
                          className={`min-w-0 truncate ${hasMultipleRemoteAddresses(forward.remoteAddr) ? "underline decoration-dotted underline-offset-2 hover:text-foreground" : "cursor-default"}`}
                          disabled={
                            !hasMultipleRemoteAddresses(forward.remoteAddr)
                          }
                          title={formatRemoteAddress(forward.remoteAddr)}
                          type="button"
                          onClick={() =>
                            showRemoteAddressModal(
                              forward.remoteAddr,
                              "出口地址",
                            )
                          }
                        >
                          {formatRemoteAddress(forward.remoteAddr)}
                        </button>
                      </div>
                    </div>
                    <div
                      className="shrink-0 text-right text-sm tabular-nums"
                      title={`上传 ${formatFlow(forward.inFlow || 0)} · 下载 ${formatFlow(forward.outFlow || 0)}`}
                    >
                      {formatFlow(calculateForwardBillingFlow(forward))}
                      <div className="text-xs text-default-400">计费</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))
        )}
      </section>

      {/* 地址列表弹窗 */}
      <Modal
        isOpen={addressModalOpen}
        placement="center"
        scrollBehavior="inside"
        size="2xl"
        onClose={() => setAddressModalOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="text-base">{addressModalTitle}</ModalHeader>
          <ModalBody className="pb-6">
            <div className="mb-4 text-right">
              <Button size="sm" onPress={copyAllAddresses}>
                复制全部
              </Button>
            </div>

            <div className="space-y-2 max-h-60 overflow-y-auto scrollbar-hide">
              {addressList.map((item) => (
                <div
                  key={item.id}
                  className="flex justify-between items-center p-3 border border-default-200 dark:border-default-100 rounded-lg"
                >
                  <code className="font-mono text-sm flex-1 mr-3 text-foreground">
                    {item.address}
                  </code>
                  <Button
                    isLoading={item.copying}
                    size="sm"
                    variant="light"
                    onPress={() => copyAddress(item)}
                  >
                    复制
                  </Button>
                </div>
              ))}
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>
    </AnimatedPage>
  );
}
