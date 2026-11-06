import type { AnnouncementData } from "@/api";

import { LazyMarkdown } from "@/components/lazy-markdown";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";

interface AnnouncementModalProps {
  announcement: AnnouncementData;
  isOpen: boolean;
  onClose: () => void;
  onDontShowAgain: () => void;
}

export const AnnouncementModal = ({
  announcement,
  isOpen,
  onClose,
  onDontShowAgain,
}: AnnouncementModalProps) => {
  return (
    <Modal
      isOpen={isOpen}
      size="2xl"
      onOpenChange={(open) => !open && onClose()}
    >
      <ModalContent>
        <ModalHeader closable className="flex flex-col gap-1">
          平台公告
        </ModalHeader>
        <ModalBody>
          <div className="max-h-[60vh] overflow-y-auto break-words text-sm leading-relaxed">
            <LazyMarkdown>{announcement.content}</LazyMarkdown>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onDontShowAgain}>
            不再提示
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
