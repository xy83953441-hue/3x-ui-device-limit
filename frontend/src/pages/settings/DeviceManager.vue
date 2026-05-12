<template>
  <div class="device-management">
    <a-row :gutter="[16, 16]" class="mb-4">
      <a-col :span="24">
        <a-typography-title :level="4">
          <SafetyOutlined /> {{ $t('pages.settings.deviceManagement') }}
        </a-typography-title>
        <a-typography-text type="secondary">
          {{ $t('pages.settings.deviceManagementDesc') }}
        </a-typography-text>
      </a-col>
    </a-row>

    <a-row :gutter="16" class="mb-4">
      <a-col :xs="24" :sm="12">
        <a-card>
          <a-form-item :label="$t('pages.settings.maxDevices')">
            <a-select v-model:value="maxDevices" style="width: 200px" @change="saveMaxDevices">
              <a-select-option :value="0">{{ $t('pages.settings.noLimit') }}</a-select-option>
              <a-select-option :value="1">1</a-select-option>
              <a-select-option :value="2">2</a-select-option>
              <a-select-option :value="3">3</a-select-option>
              <a-select-option :value="5">5</a-select-option>
              <a-select-option :value="10">10</a-select-option>
              <a-select-option :value="20">20</a-select-option>
            </a-select>
          </a-form-item>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12">
        <a-card>
          <a-statistic 
            :title="$t('pages.settings.currentDevices')" 
            :value="sessions.length"
            :suffix="$t('pages.settings.devices')"
          />
        </a-card>
      </a-col>
    </a-row>

    <a-card :title="$t('pages.settings.activeSessions')">
      <template #extra>
        <a-button 
          v-if="otherDevicesCount > 0" 
          type="primary" 
          danger 
          @click="handleKickAllOthers"
        >
          <LogoutOutlined /> {{ $t('pages.settings.kickAllOthers') }}
        </a-button>
      </template>

      <a-table 
        :columns="columns" 
        :data-source="sessions" 
        :loading="loading"
        :pagination="false"
        row-key="sessionId"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="record.isCurrent ? 'green' : 'default'">
              {{ record.isCurrent ? $t('pages.settings.current') : $t('pages.settings.other') }}
            </a-tag>
          </template>
          
          <template v-if="column.key === 'device'">
            <a-tooltip :title="record.userAgent">
              <DesktopOutlined /> 
              {{ truncateText(record.userAgent, 30) }}
            </a-tooltip>
          </template>
          
          <template v-if="column.key === 'ip'">
            <a-tag>{{ record.ipAddress }}</a-tag>
          </template>
          
          <template v-if="column.key === 'time'">
            <a-tooltip :title="record.lastSeen">
              {{ formatTime(record.lastSeen) }}
            </a-tooltip>
          </template>
          
          <template v-if="column.key === 'action'">
            <a-button 
              v-if="!record.isCurrent" 
              type="text" 
              danger 
              @click="handleKick(record)"
            >
              <LogoutOutlined /> {{ $t('pages.settings.kick') }}
            </a-button>
            <a-tag v-else color="blue">{{ $t('pages.settings.currentSession') }}</a-tag>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="kickModalVisible"
      :title="$t('pages.settings.confirmKick')"
      @ok="confirmKick"
    >
      <p>{{ $t('pages.settings.confirmKickMsg') }}</p>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { message } from 'ant-design-vue';
import {
  SafetyOutlined,
  DesktopOutlined,
  LogoutOutlined
} from '@ant-design/icons-vue';
import axios from 'axios';

const { t } = useI18n();

const loading = ref(false);
const sessions = ref([]);
const maxDevices = ref(5);
const kickModalVisible = ref(false);
const sessionToKick = ref(null);

const otherDevicesCount = computed(() => {
  return sessions.value.filter(s => !s.isCurrent).length;
});

const columns = computed(() => [
  { title: t('pages.settings.status'), key: 'status', width: 100 },
  { title: t('pages.settings.device'), key: 'device' },
  { title: t('pages.settings.ipAddress'), key: 'ip', width: 150 },
  { title: t('pages.settings.lastActive'), key: 'time', width: 180 },
  { title: t('pages.settings.actions'), key: 'action', width: 150 }
]);

const loadData = async () => {
  loading.value = true;
  try {
    const [sessionsRes, maxDevicesRes] = await Promise.all([
      axios.get('/panel/api/user/sessions'),
      axios.get('/panel/api/user/max-devices')
    ]);
    
    if (sessionsRes.data.success) {
      sessions.value = sessionsRes.data.obj;
    }
    if (maxDevicesRes.data.success) {
      maxDevices.value = maxDevicesRes.data.obj;
    }
  } catch (error) {
    console.error('Failed to load sessions:', error);
  } finally {
    loading.value = false;
  }
};

const saveMaxDevices = async () => {
  try {
    const res = await axios.put('/panel/api/user/max-devices', {
      maxDevices: maxDevices.value
    });
    if (res.data.success) {
      message.success(t('pages.settings.saveSuccess'));
    } else {
      message.error(res.data.msg);
    }
  } catch (error) {
    message.error(t('pages.settings.saveFailed'));
  }
};

const handleKick = (record) => {
  sessionToKick.value = record;
  kickModalVisible.value = true;
};

const handleKickAllOthers = () => {
  sessionToKick.value = 'all';
  kickModalVisible.value = true;
};

const confirmKick = async () => {
  try {
    if (sessionToKick.value === 'all') {
      for (const session of sessions.value) {
        if (!session.isCurrent) {
          await axios.delete(`/panel/api/user/sessions/${session.sessionId}`);
        }
      }
      message.success(t('pages.settings.kickAllSuccess'));
    } else {
      await axios.delete(`/panel/api/user/sessions/${sessionToKick.value.sessionId}`);
      message.success(t('pages.settings.kickSuccess'));
    }
    kickModalVisible.value = false;
    await loadData();
  } catch (error) {
    message.error(t('pages.settings.kickFailed'));
  }
};

const truncateText = (text, maxLength) => {
  if (!text) return '-';
  return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
};

const formatTime = (timeStr) => {
  if (!timeStr) return '-';
  const date = new Date(timeStr);
  const now = new Date();
  const diff = now - date;
  
  if (diff < 60000) return t('pages.settings.justNow');
  if (diff < 3600000) return Math.floor(diff / 60000) + ' ' + t('pages.settings.minutes');
  if (diff < 86400000) return Math.floor(diff / 3600000) + ' ' + t('pages.settings.hours');
  return timeStr;
};

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.device-management {
  padding: 24px;
}

.mb-4 {
  margin-bottom: 16px;
}
</style>
