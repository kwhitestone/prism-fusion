// 这里存放本地图标，在 src/layout/index.vue 文件中加载，避免在首启动加载
import { getSvgInfo } from "@pureadmin/utils";
import { addIcon } from "@iconify/vue/dist/offline";

// https://icon-sets.iconify.design/ep/?keyword=ep
import EpHomeFilled from "~icons/ep/home-filled?raw";
import EpDataAnalysis from "~icons/ep/data-analysis?raw";
import EpConnection from "~icons/ep/connection?raw";
import EpUpload from "~icons/ep/upload?raw";
import EpLocation from "~icons/ep/location?raw";
import EpMonitor from "~icons/ep/monitor?raw";
import EpPresent from "~icons/ep/present?raw";
import EpFiles from "~icons/ep/files?raw";
import EpGoods from "~icons/ep/goods?raw";
import EpDownload from "~icons/ep/download?raw";
import EpCopyDocument from "~icons/ep/copy-document?raw";

// https://icon-sets.iconify.design/ri/?keyword=ri
import RiSearchLine from "~icons/ri/search-line?raw";
import RiInformationLine from "~icons/ri/information-line?raw";

const icons = [
  // Element Plus Icon: https://github.com/element-plus/element-plus-icons
  ["ep/home-filled", EpHomeFilled],
  ["ep/data-analysis", EpDataAnalysis],
  ["ep/connection", EpConnection],
  ["ep/upload", EpUpload],
  ["ep/location", EpLocation],
  ["ep/monitor", EpMonitor],
  ["ep/present", EpPresent],
  ["ep/files", EpFiles],
  ["ep/goods", EpGoods],
  ["ep/download", EpDownload],
  ["ep/copy-document", EpCopyDocument],
  // Remix Icon: https://github.com/Remix-Design/RemixIcon
  ["ri/search-line", RiSearchLine],
  ["ri/information-line", RiInformationLine]
];

// 本地菜单图标，后端在路由的 icon 中返回对应的图标字符串并且前端在此处使用 addIcon 添加即可渲染菜单图标
icons.forEach(([name, icon]) => {
  addIcon(name as string, getSvgInfo(icon as string));
});
