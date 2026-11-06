import type { AnnouncementData } from "@/api";

import { LazyMarkdown } from "@/components/lazy-markdown";

interface AnnouncementBannerProps {
  announcement: AnnouncementData;
}

export const AnnouncementBanner = ({
  announcement,
}: AnnouncementBannerProps) => {
  if (!announcement.content) {
    return null;
  }

  return (
    <section className="mb-8 border-l-2 border-foreground pl-4">
      <h2 className="mb-1 text-xs font-medium text-default-500">公告</h2>
      <div className="break-words text-sm leading-relaxed">
        <LazyMarkdown>{announcement.content}</LazyMarkdown>
      </div>
    </section>
  );
};
