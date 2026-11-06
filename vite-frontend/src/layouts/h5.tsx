import {
  ArrowDownToLine,
  Globe,
  LayoutGrid,
  LineChart,
  Link2,
  Menu,
  Rss,
  Server,
} from "lucide-react";
import React, { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import toast from "react-hot-toast";

import { BrandLogo } from "@/components/brand-logo";
import { siteConfig } from "@/config/site";
import { getMonitorAccess } from "@/api";
import { getAdminFlag } from "@/utils/session";
import { useHasMySubscriptions } from "@/hooks/use-has-my-subscriptions";
import { useScrollTopOnPathChange } from "@/hooks/useScrollTopOnPathChange";

interface TabItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  adminOnly?: boolean;
  // Admins reach it from the profile page instead, to keep the bar short.
  userOnly?: boolean;
  adminLabel?: string;
}

export default function H5Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [isAdmin, setIsAdmin] = useState(() => getAdminFlag());
  const hasMySubs = useHasMySubscriptions(isAdmin);
  const [monitorAllowed, setMonitorAllowed] = useState<boolean | null>(null);
  const [monitorAccessReason, setMonitorAccessReason] = useState<string | null>(
    null,
  );

  useScrollTopOnPathChange();

  // Tabbar配置
  // Admins work along the landing → relay → subscription flow; the forward
  // pages for regular users stay as they were.
  const tabItems: TabItem[] = [
    {
      path: "/dashboard",
      label: "首页",
      adminLabel: "概览",
      icon: <LayoutGrid className="h-5 w-5" strokeWidth={1.75} />,
    },
    {
      path: "/forward",
      label: "规则",
      icon: <ArrowDownToLine className="h-5 w-5" strokeWidth={1.75} />,
      userOnly: true,
    },
    {
      path: "/my-subscriptions",
      label: "订阅",
      icon: <Rss className="h-5 w-5" strokeWidth={1.75} />,
      // Shown once the user has subscriptions assigned.
      userOnly: true,
    },
    {
      path: "/node",
      label: "服务器",
      icon: <Server className="h-5 w-5" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/inbound",
      label: "节点",
      icon: <Globe className="h-5 w-5" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/relay",
      label: "中转",
      icon: <Link2 className="h-5 w-5" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/subscription",
      label: "订阅",
      icon: <Rss className="h-5 w-5" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/monitor",
      label: "监控",
      icon: <LineChart className="h-5 w-5" strokeWidth={1.75} />,
      userOnly: true,
    },
    {
      path: "/profile",
      label: "我的",
      adminLabel: "更多",
      icon: <Menu className="h-5 w-5" strokeWidth={1.75} />,
    },
  ];

  useEffect(() => {
    const adminFlag = getAdminFlag();

    setIsAdmin(adminFlag);
    if (adminFlag) {
      setMonitorAllowed(true);
      setMonitorAccessReason(null);

      return;
    }

    let cancelled = false;

    (async () => {
      try {
        const res = await getMonitorAccess();

        if (cancelled) return;
        if (res.code === 0 && res.data) {
          setMonitorAllowed(Boolean(res.data.allowed));
          setMonitorAccessReason(
            res.data.allowed ? null : res.data.reason || null,
          );

          return;
        }
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      } catch {
        if (cancelled) return;
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  // Tab点击处理
  const handleTabClick = (path: string) => {
    if (path === "/monitor" && monitorAllowed !== true) {
      if (monitorAllowed == null) {
        toast("正在检查监控权限，请稍后重试");

        return;
      }

      const hint =
        monitorAccessReason === "need_admin_grant"
          ? "暂无监控权限，请联系管理员授权"
          : "暂无监控权限";

      toast.error(hint);

      return;
    }
    navigate(path);
  };

  // 过滤tab项（根据权限）
  const filteredTabItems = tabItems.filter(
    (item) =>
      (isAdmin ? !item.userOnly : !item.adminOnly) &&
      (item.path !== "/my-subscriptions" || hasMySubs),
  );

  return (
    <div className="flex flex-col min-h-screen bg-mesh-gradient">
      {/* 顶部导航栏 */}
      <header className="bg-background border-b border-divider h-14 safe-top flex-shrink-0 flex items-center justify-between px-4 relative z-10">
        <div className="flex items-center gap-2">
          <BrandLogo size={20} />
          <h1 className="text-sm font-bold text-foreground">
            {siteConfig.name}
          </h1>
        </div>

        <div className="flex items-center gap-2" />
      </header>

      {/* 主内容区域 */}
      <main className="flex-1">{children}</main>

      {/* 用于给固定 Tabbar 腾出空间的占位元素 */}
      <div aria-hidden className="h-[calc(4rem+var(--safe-area-bottom))]" />

      {/* 底部Tabbar */}
      <nav className="bg-background border-t border-divider h-[calc(4rem+var(--safe-area-bottom))] flex-shrink-0 flex items-center justify-around px-2 fixed bottom-0 left-0 right-0 z-30">
        {filteredTabItems.map((item) => {
          const isActive =
            location.pathname === item.path ||
            (item.path === "/relay" &&
              isAdmin &&
              ["/tunnel", "/forward"].includes(location.pathname)) ||
            (item.path === "/profile" &&
              isAdmin &&
              [
                "/monitor",
                "/user",
                "/group",
                "/limit",
                "/config",
                "/panel-sharing",
              ].includes(location.pathname));
          const isMonitor = item.path === "/monitor";
          const isMonitorBlocked = isMonitor && monitorAllowed !== true;

          return (
            <button
              key={item.path}
              className={`
                flex flex-col items-center justify-center flex-1 h-full pb-[var(--safe-area-bottom)]
                transition-colors duration-200 min-h-[44px]
                ${isMonitorBlocked ? "opacity-60" : ""}
                ${
                  isActive
                    ? "text-foreground"
                    : isMonitorBlocked
                      ? "text-gray-500 dark:text-gray-400"
                      : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                }
              `}
              onClick={() => handleTabClick(item.path)}
            >
              <div className="flex-shrink-0 mb-1">{item.icon}</div>
              <span className="text-xs font-medium">
                {(isAdmin && item.adminLabel) || item.label}
              </span>
            </button>
          );
        })}
      </nav>
    </div>
  );
}
